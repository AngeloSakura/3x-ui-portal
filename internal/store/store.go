package store

import (
	"database/sql"
	_ "embed"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Schema 仅由显式 init 命令执行；启动服务不会自动迁移数据库。
//
//go:embed schema.sql
var Schema string

type Site struct {
	Name          string `json:"name"`
	Suffix        string `json:"-"`
	PanelURL      string `json:"panelUrl"`
	PanelToken    string `json:"-"`
	SupportURL    string `json:"supportUrl"`
	AllowRotation bool   `json:"allowRotation"`
	RelayOrigins  string `json:"relayOrigins"`
}

type User struct {
	ID              int64  `json:"id"`
	Username        string `json:"username"`
	PasswordHash    string `json:"-"`
	Role            string `json:"role"`
	Disabled        bool   `json:"disabled"`
	ClientID        int64  `json:"clientId,omitempty"`
	ClientEmail     string `json:"clientEmail,omitempty"`
	CreatedAt       int64  `json:"createdAt"`
	MustSetPassword bool   `json:"mustSetPassword"`
}

func Open(dir string, initialize bool) (*sql.DB, error) {
	path, err := filepath.Abs(filepath.Join(dir, "portal.db"))
	if err != nil {
		return nil, err
	}
	_, existingErr := os.Stat(path)
	if !initialize {
		if existingErr != nil {
			return nil, errors.New("数据库不存在，请先执行 portal init")
		}
	}
	// Windows 盘符路径必须为 file:///C:/...，避免被 SQLite 误当作 URI 主机。
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := &url.URL{Scheme: "file", Path: uriPath}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Set("mode", "rw")
	if initialize {
		q.Set("mode", "rwc")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if initialize && os.IsNotExist(existingErr) {
		_, err = db.Exec(Schema)
	}
	if err == nil {
		var version int
		err = db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version)
		if err == nil && version != 2 {
			err = errors.New("数据库版本不兼容，请按 docs/sql 执行显式迁移；init 不会升级已有数据库")
		}
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func LoadSite(db *sql.DB) (*Site, error) {
	var s Site
	err := db.QueryRow("SELECT name,suffix,panel_url,panel_token,support_url,allow_rotation,relay_origins FROM site WHERE id=1").Scan(&s.Name, &s.Suffix, &s.PanelURL, &s.PanelToken, &s.SupportURL, &s.AllowRotation, &s.RelayOrigins)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

const UserColumns = "id,username,password_hash,role,disabled,COALESCE(client_id,0),COALESCE(client_email,''),created_at"

type Scanner interface{ Scan(...any) error }

func ScanUser(row Scanner) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Disabled, &u.ClientID, &u.ClientEmail, &u.CreatedAt)
	u.MustSetPassword = u.Role == "user" && u.PasswordHash == ""
	return u, err
}
