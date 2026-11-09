package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/panel"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
)

const invitationAPI = "/private-entry-123/api/"

func inviteFixture(t *testing.T) (*App, *http.Cookie) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/panel/api/clients/get" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": panel.Detail{Client: panel.Record{ID: 10, Email: "用户@example.com", Enable: true}}})
	}))
	t.Cleanup(upstream.Close)
	a, _ := accountFixture(t, upstream.URL)
	if _, err := a.db.Exec("INSERT INTO users(id,username,password_hash,role,created_at) VALUES(2,'owner','fixture','admin',0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("INSERT INTO sessions VALUES(?,2,'admin-csrf',?)", security.Digest("admin-session"), time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	return a, &http.Cookie{Name: sessionCookie, Value: "admin-session"}
}

func issueFixtureInvitation(t *testing.T, a *App, admin *http.Cookie) string {
	t.Helper()
	w := requestAccount(a, "POST", invitationAPI+"invitations", `{}`, nil, "")
	if w.Code != 401 {
		t.Fatal("anonymous invitation endpoint should require authentication")
	}
	w = requestAccount(a, "POST", invitationAPI+"admin/invitations", `{"email":"用户@example.com","clientId":10}`, admin, "admin-csrf")
	if w.Code != 200 {
		t.Fatalf("issue invitation: %d %s", w.Code, w.Body.String())
	}
	var result struct{ InvitationCode, ActivationURL, Username string }
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.InvitationCode) != 43 || result.Username != "用户@example.com" || !strings.HasSuffix(result.ActivationURL, "#activate="+result.InvitationCode) {
		t.Fatal("invitation must bind the selected panel name and return code/link")
	}
	var hash string
	var expires int64
	if err := a.db.QueryRow("SELECT token_hash,expires_at FROM activations WHERE token_hash=?", security.Digest(result.InvitationCode)).Scan(&hash, &expires); err != nil {
		t.Fatal("invitation must be stored as a digest", err)
	}
	if expires < time.Now().Add(23*time.Hour).Unix() || expires > time.Now().Add(24*time.Hour).Unix() {
		t.Fatal("invitation should expire in 24 hours")
	}
	return result.InvitationCode
}

func TestInvitationRegistersBoundPanelAccountAndRotatesSession(t *testing.T) {
	a, admin := inviteFixture(t)
	code := issueFixtureInvitation(t, a, admin)
	w := requestAccount(a, "POST", invitationAPI+"login", `{"username":"用户@example.com","password":""}`, nil, "")
	if w.Code != 401 {
		t.Fatal("panel name alone cannot claim account")
	}
	w = requestAccount(a, "POST", invitationAPI+"activate", `{"token":"`+code+`"}`, nil, "")
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("redeem: %d", w.Code)
	}
	limited := w.Result().Cookies()[0]
	w = requestAccount(a, "GET", invitationAPI+"me", "", limited, "")
	var me struct {
		User store.User `json:"user"`
		CSRF string     `json:"csrf"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil || !me.User.MustSetPassword || me.User.Username != "用户@example.com" || me.User.ClientID != 10 {
		t.Fatal("wrong restricted identity")
	}
	for _, route := range []string{"rotate", "password", "admin/invitations"} {
		if r := requestAccount(a, "POST", invitationAPI+route, `{}`, limited, me.CSRF); r.Code != 403 {
			t.Fatalf("restricted session accessed %s: %d", route, r.Code)
		}
	}
	w = requestAccount(a, "POST", invitationAPI+"first-password", `{"password":"123456"}`, limited, me.CSRF)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("set password: %d", w.Code)
	}
	full := w.Result().Cookies()[0]
	if full.Value == limited.Value || full.MaxAge != 43200 || !full.Secure || !full.HttpOnly {
		t.Fatal("must rotate to a secure normal session")
	}
	w = requestAccount(a, "GET", invitationAPI+"me", "", full, "")
	previousCSRF := me.CSRF
	if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil || w.Code != 200 || me.User.MustSetPassword || me.CSRF == previousCSRF {
		t.Fatal("new session should enter dashboard immediately with fresh CSRF")
	}
	if r := requestAccount(a, "GET", invitationAPI+"me", "", limited, ""); r.Code != 401 {
		t.Fatal("old limited session survived password setup")
	}
	if r := requestAccount(a, "POST", invitationAPI+"activate", `{"token":"`+code+`"}`, nil, ""); r.Code != 401 {
		t.Fatal("invitation reused")
	}
	if r := requestAccount(a, "POST", invitationAPI+"login", `{"username":"用户@example.com","password":"123456"}`, nil, ""); r.Code != 200 {
		t.Fatal("email/Unicode panel name cannot log in")
	}
	body, _ := json.Marshal(map[string]int64{"id": me.User.ID})
	if r := requestAccount(a, "POST", invitationAPI+"admin/invite", string(body), admin, "admin-csrf"); r.Code != 409 {
		t.Fatal("activated account downgraded to invitation")
	}
}

func TestInvitationRejectsMismatchesDuplicatesAndRequiresCSRF(t *testing.T) {
	a, admin := inviteFixture(t)
	for _, body := range []string{`{"email":"other","clientId":10}`, `{"email":"用户@example.com","clientId":11}`} {
		w := requestAccount(a, "POST", invitationAPI+"admin/invitations", body, admin, "admin-csrf")
		if w.Code == 200 {
			t.Fatal("mismatched panel identity invited")
		}
	}
	if w := requestAccount(a, "POST", invitationAPI+"admin/invitations", `{"email":"用户@example.com","clientId":10}`, admin, ""); w.Code != 403 {
		t.Fatal("invitation issuance missing CSRF check")
	}
	issueFixtureInvitation(t, a, admin)
	if w := requestAccount(a, "POST", invitationAPI+"admin/invitations", `{"email":"用户@example.com","clientId":10}`, admin, "admin-csrf"); w.Code != 409 {
		t.Fatal("duplicate binding created")
	}
}

func TestInvitationReissueRevokesOldCodeAndLimitedSession(t *testing.T) {
	a, admin := inviteFixture(t)
	code := issueFixtureInvitation(t, a, admin)
	w := requestAccount(a, "POST", invitationAPI+"activate", `{"token":"`+code+`"}`, nil, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	limited := w.Result().Cookies()[0]
	var id int64
	if err := a.db.QueryRow("SELECT id FROM users WHERE client_id=10").Scan(&id); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]int64{"id": id})
	w = requestAccount(a, "POST", invitationAPI+"admin/invite", string(body), admin, "admin-csrf")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if r := requestAccount(a, "GET", invitationAPI+"me", "", limited, ""); r.Code != 401 {
		t.Fatal("stale session accepted")
	}
	if r := requestAccount(a, "POST", invitationAPI+"activate", `{"token":"`+code+`"}`, nil, ""); r.Code != 401 {
		t.Fatal("old code accepted")
	}
	var next struct{ InvitationCode string }
	if err := json.Unmarshal(w.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("UPDATE users SET disabled=1 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if r := requestAccount(a, "POST", invitationAPI+"activate", `{"token":"`+next.InvitationCode+`"}`, nil, ""); r.Code != 401 {
		t.Fatal("disabled account redeemed")
	}
	if _, err := a.db.Exec("UPDATE users SET disabled=0 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("UPDATE activations SET expires_at=0 WHERE user_id=?", id); err != nil {
		t.Fatal(err)
	}
	if r := requestAccount(a, "POST", invitationAPI+"activate", `{"token":"`+next.InvitationCode+`"}`, nil, ""); r.Code != 401 {
		t.Fatal("expired code redeemed")
	}
}

func TestInvitationConcurrentRedemptionAndRateLimit(t *testing.T) {
	a, admin := inviteFixture(t)
	code := issueFixtureInvitation(t, a, admin)
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- requestAccount(a, "POST", invitationAPI+"activate", `{"token":"`+code+`"}`, nil, "").Code
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for status := range results {
		if status == 200 {
			success++
		} else if status != 401 {
			t.Fatal(status)
		}
	}
	if success != 1 {
		t.Fatal("code must be consumed exactly once")
	}
	for i := 0; i < 18; i++ {
		if w := requestAccount(a, "POST", invitationAPI+"activate", `{"token":"invalid"}`, nil, ""); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	if w := requestAccount(a, "POST", invitationAPI+"activate", `{"token":"invalid"}`, nil, ""); w.Code != 429 {
		t.Fatal("redemption not rate limited")
	}
}
