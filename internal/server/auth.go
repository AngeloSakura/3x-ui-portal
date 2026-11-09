package server

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "portal_session"

func (a *App) cookie(c *gin.Context, token string, age int) {
	site, _ := a.snapshot()
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: token, Path: "/" + site.Suffix + "/", MaxAge: age, HttpOnly: true, Secure: a.cfg.Secure, SameSite: http.SameSiteStrictMode})
}

func (a *App) login(c *gin.Context) {
	if !a.limiter.Allow("login-global", 60, time.Minute) || !a.limiter.Allow("login-ip:"+c.ClientIP(), 40, 15*time.Minute) {
		fail(c, 429, "登录尝试过于频繁，请 15 分钟后重试")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(c, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if !security.ValidLoginName(in.Username) || len(in.Password) > 72 {
		fail(c, 401, "账号或密码不正确")
		return
	}
	if !a.limiter.Allow("login-name:"+strings.ToLower(in.Username), 10, 15*time.Minute) {
		fail(c, 429, "此账号登录尝试过多，请 15 分钟后重试")
		return
	}
	u, err := store.ScanUser(a.db.QueryRow("SELECT "+store.UserColumns+" FROM users WHERE username=?", in.Username))
	if err != nil || u.MustSetPassword {
		// 不存在的账号也执行同成本的比较，减少账号枚举时序差异。
		var dummy string
		_ = a.db.QueryRow("SELECT password_hash FROM users WHERE role='admin' LIMIT 1").Scan(&dummy)
		_ = bcrypt.CompareHashAndPassword([]byte(dummy), []byte(in.Password))
		fail(c, 401, "账号或密码不正确")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil || u.Disabled {
		fail(c, 401, "账号或密码不正确")
		return
	}
	token, err := security.Random(32)
	if err != nil {
		fail(c, 500, "无法建立会话")
		return
	}
	csrf, err := security.Random(32)
	if err != nil {
		fail(c, 500, "无法建立会话")
		return
	}
	_, _ = a.db.Exec("DELETE FROM sessions WHERE expires_at<=?", time.Now().Unix())
	res, err := a.db.Exec("INSERT INTO sessions(token_hash,user_id,csrf,expires_at) SELECT ?,id,?,? FROM users WHERE id=? AND password_hash=? AND disabled=0", security.Digest(token), csrf, time.Now().Add(12*time.Hour).Unix(), u.ID, u.PasswordHash)
	if err != nil {
		fail(c, 500, "无法保存登录会话，请重试")
		return
	}
	if n, e := res.RowsAffected(); e != nil || n != 1 {
		fail(c, 401, "账户状态已改变，请重新登录")
		return
	}
	a.cookie(c, token, 12*60*60)
	a.audit(u.ID, "login", u.ID, "success")
	ok(c, gin.H{"message": "登录成功"})
}

func (a *App) session(c *gin.Context) (store.User, string, bool) {
	var u store.User
	var csrf string
	token, err := c.Cookie(sessionCookie)
	if err != nil || len(token) > 128 {
		fail(c, 401, "请先登录")
		return u, csrf, false
	}
	var id int64
	err = a.db.QueryRow("SELECT user_id,csrf FROM sessions WHERE token_hash=? AND expires_at>?", security.Digest(token), time.Now().Unix()).Scan(&id, &csrf)
	if err == nil {
		u, err = store.ScanUser(a.db.QueryRow("SELECT "+store.UserColumns+" FROM users WHERE id=?", id))
	}
	if err != nil || u.Disabled {
		fail(c, 401, "登录已失效，请重新登录")
		return u, csrf, false
	}
	return u, csrf, true
}

func (a *App) logout(c *gin.Context) {
	token, _ := c.Cookie(sessionCookie)
	if _, err := a.db.Exec("DELETE FROM sessions WHERE token_hash=?", security.Digest(token)); err != nil {
		fail(c, 500, "退出失败，请重试")
		return
	}
	a.cookie(c, "", -1)
	ok(c, gin.H{"message": "已退出"})
}

func (a *App) password(c *gin.Context, u store.User) {
	if !a.limiter.Allow("password:"+u.Username, 5, 15*time.Minute) {
		fail(c, 429, "尝试过于频繁，请稍后重试")
		return
	}
	var in struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !decode(c, &in) {
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Current)) != nil {
		fail(c, 400, "当前密码不正确")
		return
	}
	hash, err := security.HashPassword(in.Password)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(c, 500, "密码更新失败")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE users SET password_hash=? WHERE id=? AND password_hash=? AND disabled=0", hash, u.ID, u.PasswordHash)
	if err == nil {
		var n int64
		n, err = res.RowsAffected()
		if n != 1 {
			err = sql.ErrNoRows
		}
	}
	if err == nil {
		_, err = tx.Exec("DELETE FROM sessions WHERE user_id=?", u.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(c, 409, "账号状态已改变，请重新登录")
		return
	}
	a.cookie(c, "", -1)
	a.audit(u.ID, "password-change", u.ID, "success")
	ok(c, gin.H{"message": "密码已更新，请重新登录"})
}
