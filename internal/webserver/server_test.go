package webserver

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"meshconsole/internal/auth"
	"meshconsole/internal/captcha"
	"meshconsole/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestGateAndStatic(t *testing.T) {
	hash, _ := auth.HashPassword("integration-password")
	cfg := config.Config{Host: "127.0.0.1", Port: 3080, Origin: "https://console.example.com", Username: "admin", PasswordHash: hash, Demo: true, SessionTTL: time.Hour, LoginRateLimit: 10, LoginRateWindow: time.Hour, PolicyMode: "file"}
	front := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<h1>Mesh</h1>")}, "assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")}}
	h, e := New(cfg, fs.FS(front), auth.WithCaptchaFactory(func() (string, []byte, error) { return captcha.Generate("TEST") }))
	if e != nil {
		t.Fatal(e)
	}
	ts := httptest.NewServer(h)
	defer ts.Close()
	req := func(method, p, body, cookie, csrf string) *http.Response {
		r, _ := http.NewRequest(method, ts.URL+p, strings.NewReader(body))
		r.Header.Set("Origin", cfg.Origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Cookie", cookie)
		r.Header.Set("X-CSRF-Token", csrf)
		res, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return res
	}
	for _, p := range []string{"/api/devices", "/api/config/dns", "/api/connection"} {
		r := req("GET", p, "", "", "")
		if r.StatusCode != 401 {
			t.Fatalf("%s unguarded", p)
		}
		r.Body.Close()
	}
	r := req("POST", "/api/auth/captcha", "{}", "", "")
	var ch map[string]any
	_ = json.NewDecoder(r.Body).Decode(&ch)
	r.Body.Close()
	cookie := r.Cookies()[0].Name + "=" + r.Cookies()[0].Value
	body, _ := json.Marshal(map[string]any{"username": "admin", "password": "integration-password", "captchaId": ch["captchaId"], "captcha": "TEST"})
	r = req("POST", "/api/auth/login", string(body), cookie, "")
	if r.StatusCode != 200 {
		t.Fatal(r.Status)
	}
	var s map[string]any
	_ = json.NewDecoder(r.Body).Decode(&s)
	r.Body.Close()
	cookie = r.Cookies()[0].Name + "=" + r.Cookies()[0].Value
	r = req("POST", "/api/config/dns/apply", `{"confirmed":true,"token":"bad"}`, cookie, "")
	if r.StatusCode != 403 {
		t.Fatal("CSRF bypass")
	}
	r.Body.Close()
	r = req("GET", "/api/connection", "", cookie, "")
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || !bytes.Contains(b, []byte(`"mode":"demo"`)) {
		t.Fatal(string(b))
	}
	for _, p := range []string{"/", "/devices/1"} {
		r = req("GET", p, "", "", "")
		b, _ = io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != 200 || !bytes.Contains(b, []byte("Mesh")) {
			t.Fatal(p)
		}
	}
	r = req("GET", "/assets/missing.js", "", "", "")
	if r.StatusCode != 404 {
		t.Fatal("missing asset fallback")
	}
	r.Body.Close()
	r = req("GET", "/%2e%2e%2fserver/secret", "", "", "")
	if r.StatusCode != 400 {
		t.Fatal("traversal")
	}
	r.Body.Close()
}
