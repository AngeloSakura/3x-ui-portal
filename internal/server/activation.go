package server

import (
	"database/sql"
	"errors"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
	"github.com/gin-gonic/gin"
)

// 站长只为未设置密码的用户重发邀请；已激活账户不能降级回免密码状态。
func (a *App) invite(c *gin.Context, actor store.User) {
	if !a.limiter.Allow("invite", 30, time.Minute) {
		fail(c, 429, "操作频繁，请稍后再试")
		return
	}
	var in struct {
		ID int64 `json:"id"`
	}
	if !decode(c, &in) {
		return
	}
	token, err := security.Random(32)
	if err != nil {
		fail(c, 500, "无法生成激活链接")
		return
	}
	tx, err := a.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		fail(c, 500, "无法签发激活链接")
		return
	}
	defer tx.Rollback()
	var id int64
	var username string
	err = tx.QueryRow("SELECT id,username FROM users WHERE id=? AND role='user' AND password_hash='' AND disabled=0", in.ID).Scan(&id, &username)
	if err != nil {
		fail(c, 409, "只能为启用且尚未设置密码的用户签发激活链接")
		return
	}
	expires := time.Now().Add(24 * time.Hour).Unix()
	_, err = tx.Exec("INSERT INTO activations(user_id,token_hash,expires_at) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET token_hash=excluded.token_hash,expires_at=excluded.expires_at", id, security.Digest(token), expires)
	if err == nil {
		_, err = tx.Exec("DELETE FROM sessions WHERE user_id=?", id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(c, 500, "激活链接签发失败，请重试")
		return
	}
	a.audit(actor.ID, "activation-issue", id, "success")
	a.inviteResult(c, token, expires, username)
}

func (a *App) inviteResult(c *gin.Context, token string, expires int64, username string) {
	site, _ := a.snapshot()
	// fragment 不会发送到 Web 服务器访问日志，页面读取后立即从地址栏清除。
	ok(c, gin.H{"activationUrl": a.cfg.Origin + "/" + site.Suffix + "/#activate=" + token, "expiresAt": expires, "username": username})
}

func (a *App) activate(c *gin.Context) {
	if !a.limiter.Allow("activate-global", 60, time.Minute) {
		fail(c, 429, "激活尝试过于频繁")
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !decode(c, &in) {
		return
	}
	if len(in.Token) != 43 {
		fail(c, 401, "激活链接无效或已过期，请联系站长重新签发")
		return
	}
	token, err := security.Random(32)
	if err != nil {
		fail(c, 500, "无法建立激活会话")
		return
	}
	csrf, err := security.Random(32)
	if err != nil {
		fail(c, 500, "无法建立激活会话")
		return
	}
	tx, err := a.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		fail(c, 500, "暂时无法激活，请重试")
		return
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow("SELECT a.user_id FROM activations a JOIN users u ON u.id=a.user_id WHERE a.token_hash=? AND a.expires_at>? AND u.role='user' AND u.password_hash='' AND u.disabled=0", security.Digest(in.Token), time.Now().Unix()).Scan(&id)
	if err != nil {
		fail(c, 401, "激活链接无效或已过期，请联系站长重新签发")
		return
	}
	res, err := tx.Exec("DELETE FROM activations WHERE user_id=? AND token_hash=?", id, security.Digest(in.Token))
	if err == nil {
		var n int64
		n, err = res.RowsAffected()
		if n != 1 {
			err = sql.ErrNoRows
		}
	}
	if err == nil {
		_, err = tx.Exec("DELETE FROM sessions WHERE user_id=?", id)
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO sessions(token_hash,user_id,csrf,expires_at) VALUES(?,?,?,?)", security.Digest(token), id, csrf, time.Now().Add(10*time.Minute).Unix())
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(c, 409, "激活未完成，请重试或联系站长")
		return
	}
	a.cookie(c, token, 10*60)
	a.audit(id, "activation-start", id, "success")
	ok(c, gin.H{"message": "请设置本站登录密码后继续"})
}

func (a *App) firstPassword(c *gin.Context, u store.User) {
	if !u.MustSetPassword {
		fail(c, 409, "账户已设置密码，请使用修改密码功能")
		return
	}
	if !a.limiter.Allow("first-password:"+u.Username, 5, 10*time.Minute) {
		fail(c, 429, "提交频繁，请稍后再试")
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decode(c, &in) {
		return
	}
	hash, err := security.HashPassword(in.Password)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	cookie, _ := c.Cookie(sessionCookie)
	tx, err := a.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		fail(c, 500, "密码保存失败")
		return
	}
	defer tx.Rollback()
	// 再次核实有限会话，防止重发激活链接后仍使用旧的在途请求设置密码。
	res, err := tx.Exec("UPDATE users SET password_hash=? WHERE id=? AND role='user' AND password_hash='' AND disabled=0 AND EXISTS(SELECT 1 FROM sessions WHERE token_hash=? AND user_id=? AND expires_at>?)", hash, u.ID, security.Digest(cookie), u.ID, time.Now().Unix())
	if err == nil {
		n, e := res.RowsAffected()
		err = e
		if n != 1 {
			err = errors.New("activation changed")
		}
	}
	if err == nil {
		_, err = tx.Exec("DELETE FROM sessions WHERE user_id=?", u.ID)
	}
	if err == nil {
		_, err = tx.Exec("DELETE FROM activations WHERE user_id=?", u.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(c, 409, "激活状态已变化，请重新登录或联系站长")
		return
	}
	a.cookie(c, "", -1)
	a.audit(u.ID, "activation-complete", u.ID, "success")
	ok(c, gin.H{"message": "密码已设置，请使用账号和新密码登录"})
}
