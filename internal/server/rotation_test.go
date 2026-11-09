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
)

func TestRotationMinimalWriteReadbackAndAmbiguousOutcome(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		name := "confirmed"
		if ambiguous {
			name = "ambiguous"
		}
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			writes := 0
			record := panel.Record{ID: 9, Email: "client-a", SubID: "original-sub", UUID: "preserve-node-credential", Enable: true, Expiry: time.Now().Add(time.Hour).UnixMilli(), Total: 10000}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				var obj any
				switch {
				case r.URL.Path == "/panel/api/clients/get":
					obj = panel.Detail{Client: record, InboundIDs: []int64{1}}
				case strings.Contains(r.URL.Path, "/clients/traffic/"):
					obj = panel.Traffic{Email: record.Email, Enable: true, Expiry: record.Expiry, Total: record.Total}
				case strings.Contains(r.URL.Path, "/clients/get/subId/"):
					obj = []panel.Detail{{Client: record, InboundIDs: []int64{1}}}
				case r.URL.Path == "/panel/api/inbounds/get/1":
					obj = map[string]any{"id": 1, "enable": true}
				case r.URL.Path == "/panel/api/openapi.json":
					_ = json.NewEncoder(w).Encode(map[string]any{"paths": map[string]any{"/clients/update/{email}": map[string]any{"post": map[string]string{"description": "omitted fields keep their current values"}}}})
					return
				case r.URL.Path == "/panel/api/clients/update" && r.Method == "POST":
					writes++
					var body map[string]string
					if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
						t.Error(e)
					}
					if len(body) != 1 || body["subId"] == "" || r.URL.Query().Get("email") != record.Email {
						t.Error("write contained unexpected fields or target")
					}
					record.SubID = body["subId"]
					if ambiguous {
						_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
						return
					}
				default:
					t.Errorf("unexpected panel request %s", r.URL.Path)
					w.WriteHeader(404)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": obj})
			}))
			defer upstream.Close()
			app, u := accountFixture(t, upstream.URL)
			hash, e := security.HashPassword("fixture-password-long")
			if e != nil {
				t.Fatal(e)
			}
			if _, e = app.db.Exec("UPDATE users SET password_hash=? WHERE id=?", hash, u.ID); e != nil {
				t.Fatal(e)
			}
			site, _ := app.snapshot()
			copy := *site
			copy.AllowRotation = true
			if e = app.configure(&copy); e != nil {
				t.Fatal(e)
			}
			prefix := "/private-entry-123/api/"
			login := requestAccount(app, "POST", prefix+"login", `{"username":"member","password":"fixture-password-long"}`, nil, "")
			if login.Code != 200 {
				t.Fatalf("login: %d %s", login.Code, login.Body.String())
			}
			cookie := login.Result().Cookies()[0]
			me := requestAccount(app, "GET", prefix+"me", "", cookie, "")
			var session struct {
				CSRF string `json:"csrf"`
			}
			_ = json.Unmarshal(me.Body.Bytes(), &session)
			result := requestAccount(app, "POST", prefix+"rotate", `{"password":"fixture-password-long"}`, cookie, session.CSRF)
			expected := 200
			expectedState := "confirmed"
			if ambiguous {
				expected = 409
				expectedState = "review"
			}
			if result.Code != expected {
				t.Fatalf("rotate: %d %s", result.Code, result.Body.String())
			}
			var state string
			if e = app.db.QueryRow("SELECT status FROM rotations WHERE user_id=?", u.ID).Scan(&state); e != nil || state != expectedState {
				t.Fatalf("state: %q %v", state, e)
			}
			again := requestAccount(app, "POST", prefix+"rotate", `{"password":"fixture-password-long"}`, cookie, session.CSRF)
			if again.Code == 200 {
				t.Fatal("immediate retry accepted")
			}
			mu.Lock()
			defer mu.Unlock()
			if writes != 1 || record.UUID != "preserve-node-credential" || record.SubID == "original-sub" {
				t.Fatal("write count or preserved fields incorrect")
			}
		})
	}
}
