package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Listen, Origin, DataDir, KeyFile, WebDir string
	Secure                                   bool
	TLSMode, ChallengeListen                 string
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func Load() (Config, error) {
	c := Config{Listen: env("PORTAL_LISTEN", "127.0.0.1:8090"), Origin: strings.TrimRight(env("PORTAL_ORIGIN", "http://localhost:8090"), "/"), DataDir: env("PORTAL_DATA_DIR", "./data"), KeyFile: env("PORTAL_KEY_FILE", "./secrets/master.key"), WebDir: env("PORTAL_WEB_DIR", "./frontend/dist")}
	c.TLSMode = env("PORTAL_TLS_MODE", "proxy")
	c.ChallengeListen = env("PORTAL_ACME_LISTEN", "0.0.0.0:8080")
	if c.TLSMode != "proxy" && c.TLSMode != "managed" {
		return c, errors.New("PORTAL_TLS_MODE 仅支持 proxy 或 managed")
	}
	u, err := url.Parse(c.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return c, errors.New("PORTAL_ORIGIN 必须是无路径的公开源地址")
	}
	c.Secure = u.Scheme == "https"
	if !c.Secure && (u.Scheme != "http" || !loopback(u.Hostname())) {
		return c, errors.New("非回环公开地址必须使用 HTTPS")
	}
	h, _, err := net.SplitHostPort(c.Listen)
	if err != nil || (c.TLSMode == "proxy" && !loopback(h)) {
		return c, errors.New("后端只允许监听回环地址；请通过同机 HTTPS 反向代理访问")
	}
	if c.TLSMode == "managed" && !c.Secure {
		return c, errors.New("托管证书模式必须使用 HTTPS 公开地址")
	}
	if _, _, err = net.SplitHostPort(c.ChallengeListen); err != nil {
		return c, errors.New("PORTAL_ACME_LISTEN 格式错误")
	}
	return c, nil
}

func loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func WriteSecret(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
