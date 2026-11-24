package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/gif"
	"meshconsole/internal/captcha"
	"meshconsole/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const nodeHash = "scrypt$16384$8$1$BwcHBwcHBwcHBwcHBwcHBw==$ITVJINRlwVHQkq7R/OIygorvwc5zydc+Y/lRZx55NtPI27cqgUNidOLB4IkS42i3xJ92AU813Blyn0XCW8E7mA=="

func TestNodeHashCompatibility(t *testing.T) {
	good, e := VerifyPassword("NodeCompatible-password!", nodeHash)
	if e != nil || !good {
		t.Fatalf("Node vector failed: %v", e)
	}
	bad, _ := VerifyPassword("wrong", nodeHash)
	if bad {
		t.Fatal("wrong password accepted")
	}
	hash, e := HashPassword("new-password-123")
	if e != nil {
		t.Fatal(e)
	}
	good, e = VerifyPassword("new-password-123", hash)
	if e != nil || !good {
		t.Fatal("roundtrip failed")
	}
}

type fixture struct {
	m   *Manager
	t   *testing.T
	now time.Time
}

func setup(t *testing.T) *fixture {
	f := &fixture{t: t, now: time.Now()}
	cfg := config.Config{Username: "admin", PasswordHash: nodeHash, Origin: "http://127.0.0.1:3080", SessionTTL: time.Hour, LoginRateLimit: 100, LoginRateWindow: time.Minute}
	f.m = New(cfg, WithNow(func() time.Time { return f.now }), WithCaptchaFactory(func() (string, []byte, error) { return captcha.Generate("TEST") }))
	return f
}
func (f *fixture) request(path, method string, b any, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	var data []byte
	if b != nil {
		data, _ = json.Marshal(b)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.RemoteAddr = "127.0.0.1:5678"
	r.Header.Set("Origin", "http://127.0.0.1:3080")
	r.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	if !f.m.Handle(w, r) {
		_, ok := f.m.Require(w, r)
		if ok {
			jsonReply(w, 200, map[string]bool{"allowed": true})
		}
	}
	return w
}
func bodyMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var v map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func (f *fixture) claim(cookies []*http.Cookie) (string, []*http.Cookie) {
	w := f.request("/api/auth/captcha", "POST", map[string]any{}, cookies, nil)
	if w.Code != 200 {
		f.t.Fatalf("claim=%d %s", w.Code, w.Body.String())
	}
	b := bodyMap(f.t, w)
	return b["captchaId"].(string), w.Result().Cookies()
}
func (f *fixture) login(id string, cookies []*http.Cookie, answer, password string) *httptest.ResponseRecorder {
	return f.request("/api/auth/login", "POST", map[string]string{"username": "admin", "password": password, "captchaId": id, "captcha": answer}, cookies, nil)
}
func TestCaptchaPrivacyOriginAndGIF(t *testing.T) {
	f := setup(t)
	for _, origin := range []string{"", "https://evil.test"} {
		w := f.request("/api/auth/captcha", "POST", nil, nil, map[string]string{"Origin": origin, "X-Forwarded-Host": "127.0.0.1:3080"})
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	w := f.request("/api/auth/captcha", "POST", nil, nil, nil)
	b := bodyMap(t, w)
	if len(b) != 3 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("challenge privacy")
	}
	data, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(b["image"].(string), "data:image/gif;base64,"))
	if e != nil {
		t.Fatal(e)
	}
	g, e := gif.DecodeAll(bytes.NewReader(data))
	if e != nil || len(g.Image) != 4 {
		t.Fatalf("invalid gif %v", e)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 120 {
		t.Fatal("cookie flags")
	}
	if Date := b["expiresAt"].(string); Date == "" {
		t.Fatal("expiry")
	}
}
func TestCaptchaMissingWrongReplayAndExpiry(t *testing.T) {
	f := setup(t)
	w := f.login("", nil, "", "NodeCompatible-password!")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	id, c := f.claim(nil)
	if f.login(id, c, "WRNG", "NodeCompatible-password!").Code != 400 {
		t.Fatal("wrong accepted")
	}
	if f.login(id, c, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("replay accepted")
	}
	id, c = f.claim(c)
	f.now = f.now.Add(120 * time.Second)
	if f.login(id, c, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("expiry accepted")
	}
	id, c = f.claim(c)
	if f.login(id, c, " test ", "NodeCompatible-password!").Code != 200 {
		t.Fatal("valid rejected")
	}
	if f.login(id, c, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("successful replay")
	}
}
func TestCaptchaCookieBindingRotationAndFailedPassword(t *testing.T) {
	f := setup(t)
	id, a := f.claim(nil)
	other, b := f.claim(nil)
	if f.login(other, a, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("cookie crossover")
	}
	if f.login(other, b, "TEST", "NodeCompatible-password!").Code != 200 {
		t.Fatal("other damaged")
	}
	id, a = f.claim(a)
	second, a := f.claim(a)
	if f.login(id, a, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("old id accepted")
	}
	if f.login(second, a, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("old attempt must consume")
	}
	id, a = f.claim(a)
	if f.login(id, a, "TEST", "wrong").Code != 401 {
		t.Fatal("wrong password")
	}
	if f.login(id, a, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("wrong password replay")
	}
	id, a = f.claim(a)
	if f.login(id, nil, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("missing cookie accepted")
	}
}
func TestSessionCSRFLogoutFixedExpiry(t *testing.T) {
	f := setup(t)
	id, pre := f.claim(nil)
	w := f.login(id, pre, "TEST", "NodeCompatible-password!")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	session := w.Result().Cookies()
	v := bodyMap(t, w)
	csrf := v["csrfToken"].(string)
	if len(csrf) != 64 || !session[0].HttpOnly || session[0].MaxAge != 3600 {
		t.Fatal("session contract")
	}
	if _, ok := v["token"]; ok {
		t.Fatal("token leaked")
	}
	if f.request("/api/auth/session", "GET", nil, session, nil).Code != 200 {
		t.Fatal("session restore")
	}
	for _, h := range []map[string]string{nil, {"X-CSRF-Token": strings.Repeat("0", 64)}, {"X-CSRF-Token": csrf, "Origin": "https://evil.test"}} {
		if f.request("/api/business", "POST", nil, session, h).Code != 403 {
			t.Fatal("csrf accepted")
		}
	}
	if f.request("/api/business", "POST", nil, session, map[string]string{"X-CSRF-Token": csrf}).Code != 200 {
		t.Fatal("valid csrf")
	}
	f.now = f.now.Add(time.Hour)
	if f.request("/api/auth/session", "GET", nil, session, nil).Code != 401 {
		t.Fatal("expiry renewed")
	}
	id, pre = f.claim(pre)
	w = f.login(id, pre, "TEST", "NodeCompatible-password!")
	session = w.Result().Cookies()
	csrf = bodyMap(t, w)["csrfToken"].(string)
	logout := f.request("/api/auth/logout", "POST", nil, session, map[string]string{"X-CSRF-Token": csrf})
	if logout.Code != 200 || logout.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout")
	}
	if f.request("/api/auth/session", "GET", nil, session, nil).Code != 401 {
		t.Fatal("logout replay")
	}
}
func TestRateLimitsAndSecureCookie(t *testing.T) {
	f := setup(t)
	f.m.cfg.LoginRateLimit = 2
	for i := 0; i < 3; i++ {
		w := f.login("", nil, "", "bad")
		if i == 2 && w.Code != 429 {
			t.Fatal("login rate limit")
		}
	}
	for i := 0; i < 21; i++ {
		w := f.request("/api/auth/captcha", "POST", nil, nil, map[string]string{"X-Forwarded-For": "192.0.2." + string(rune('a'+i))})
		if i == 20 && (w.Code != 429 || w.Header().Get("Retry-After") == "") {
			t.Fatal("captcha rate limit")
		}
	}
	f = setup(t)
	f.m.cfg.SecureCookie = true
	id, c := f.claim(nil)
	if !c[0].Secure || c[0].Name != "__Host-mesh_captcha" {
		t.Fatal("secure pre cookie")
	}
	w := f.login(id, c, "TEST", "NodeCompatible-password!")
	if w.Code != 200 || w.Result().Cookies()[0].Name != "__Host-mesh_session" {
		t.Fatal("secure session")
	}
}
func TestHandleScopeAndFailClosed(t *testing.T) {
	f := setup(t)
	r := httptest.NewRequest("GET", "/other", nil)
	if f.m.Handle(httptest.NewRecorder(), r) {
		t.Fatal("unrelated path claimed")
	}
	m := New(config.Config{})
	w := httptest.NewRecorder()
	m.Handle(w, httptest.NewRequest("POST", "/api/auth/login", nil))
	if w.Code != 503 {
		t.Fatal("invalid config fail open")
	}
	if f.request("/api/business", "POST", nil, nil, nil).Code != 401 {
		t.Fatal("unauthed business allowed")
	}
}
