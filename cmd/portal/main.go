package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/certmgr"
	"github.com/AngeloSakura/3x-ui-portal/internal/config"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/server"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return errors.New("用法：portal init | serve | tls-config | tls-issue | tls-import | tls-status")
	}
	command := os.Args[1]
	if command != "init" && command != "serve" && command != "tls-config" && command != "tls-issue" && command != "tls-import" && command != "tls-status" {
		return errors.New("未知命令")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupPath := filepath.Join(cfg.DataDir, "setup.token")
	if os.Args[1] == "init" {
		if err = os.MkdirAll(cfg.DataDir, 0700); err != nil {
			return err
		}
		// 数据库存在时绝不新建替代主密钥，防止旧密文永久不可读。
		if _, e := os.Stat(cfg.KeyFile); os.IsNotExist(e) {
			if _, d := os.Stat(filepath.Join(cfg.DataDir, "portal.db")); !os.IsNotExist(d) {
				return errors.New("数据库已存在但主密钥丢失，请从备份恢复")
			}
			key := make([]byte, 32)
			if _, err = rand.Read(key); err != nil {
				return err
			}
			if err = config.WriteSecret(cfg.KeyFile, key); err != nil {
				return err
			}
		}
		key, e := os.ReadFile(cfg.KeyFile)
		if e != nil {
			return e
		}
		if _, e = security.NewVault(key); e != nil {
			return e
		}
		db, e := store.Open(cfg.DataDir, true)
		if e != nil {
			return e
		}
		defer db.Close()
		site, e := store.LoadSite(db)
		if e != nil {
			return e
		}
		if site != nil {
			return errors.New("本站已安装；init 不会重置入口或管理员")
		}
		if _, e = os.Stat(setupPath); os.IsNotExist(e) {
			token, e := security.Random(32)
			if e != nil {
				return e
			}
			if e = config.WriteSecret(setupPath, []byte(token)); e != nil {
				return e
			}
		}
		fmt.Println("独立数据库初始化完成。启动 serve 后，在本机读取", setupPath, "，访问公开地址 /install/凭证/ 完成安装。")
		return nil
	}
	key, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return errors.New("缺少主密钥；首次部署请先执行 init，已有安装请恢复备份")
	}
	vault, err := security.NewVault(key)
	if err != nil {
		return err
	}
	u, _ := url.Parse(cfg.Origin)
	certificates, err := certmgr.New(cfg.DataDir, u.Hostname(), cfg.ChallengeListen, vault)
	if err != nil {
		return err
	}
	switch command {
	case "tls-config":
		var s certmgr.Settings
		d := json.NewDecoder(io.LimitReader(os.Stdin, 16<<10))
		d.DisallowUnknownFields()
		if err = d.Decode(&s); err != nil {
			return errors.New("请从标准输入提供证书配置 JSON")
		}
		return certificates.Configure(s)
	case "tls-issue":
		return certificates.Issue(context.Background())
	case "tls-import":
		cert, e := os.ReadFile(os.Getenv("PORTAL_IMPORT_CERT"))
		if e != nil {
			return errors.New("无法读取 PORTAL_IMPORT_CERT")
		}
		key, e := os.ReadFile(os.Getenv("PORTAL_IMPORT_KEY"))
		if e != nil {
			return errors.New("无法读取 PORTAL_IMPORT_KEY")
		}
		return certificates.Import(cert, key)
	case "tls-status":
		return json.NewEncoder(os.Stdout).Encode(certificates.Status())
	}
	db, err := store.Open(cfg.DataDir, false)
	if err != nil {
		return err
	}
	defer db.Close()
	app, err := server.New(cfg, db, vault)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: cfg.Listen, Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.TLSMode == "managed" {
		if _, err = certificates.GetCertificate(nil); err != nil {
			return err
		}
		app.SetCertificates(certificates)
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: certificates.GetCertificate}
		go certificates.Run(ctx)
	}
	errCh := make(chan error, 1)
	go func() {
		if cfg.TLSMode == "managed" {
			errCh <- srv.ListenAndServeTLS("", "")
		} else {
			errCh <- srv.ListenAndServe()
		}
	}()
	log.Printf("门户已监听 %s；不在日志中显示秘密入口", cfg.Listen)
	select {
	case err = <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(closeCtx)
}
