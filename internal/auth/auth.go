// Package auth implements server-side opaque sessions and single-use captcha login.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"meshconsole/internal/captcha"
	"meshconsole/internal/config"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Session struct {
	Username  string    `json:"username"`
	CSRFToken string    `json:"csrfToken"`
	ExpiresAt time.Time `json:"expiresAt"`
	token     string
}
type challenge struct {
	id      string
	digest  []byte
	expires time.Time
}
type attempt struct {
	count int
	until time.Time
}
type CaptchaFactory func() (string, []byte, error)
type Option func(*Manager)

func WithCaptchaFactory(f CaptchaFactory) Option {
	return func(m *Manager) {
		if f != nil {
			m.factory = f
		}
	}
}
func WithNow(f func() time.Time) Option {
	return func(m *Manager) {
		if f != nil {
			m.now = f
		}
	}
}

type Manager struct {
	cfg             config.Config
	mu              sync.Mutex
	sessions        map[string]Session
	challenges      map[string]challenge
	loginAttempts   map[string]attempt
	captchaAttempts map[string]attempt
	key             []byte
	factory         CaptchaFactory
	now             func() time.Time
	scryptSlots     chan struct{}
	valid           bool
}

func New(cfg config.Config, options ...Option) *Manager {
	m := &Manager{cfg: cfg, sessions: map[string]Session{}, challenges: map[string]challenge{}, loginAttempts: map[string]attempt{}, captchaAttempts: map[string]attempt{}, key: make([]byte, 32), factory: func() (string, []byte, error) { return captcha.Generate("") }, now: time.Now, scryptSlots: make(chan struct{}, 4)}
	_, e := rand.Read(m.key)
	m.valid = e == nil && cfg.Username != "" && config.ValidatePasswordHash(cfg.PasswordHash) == nil && cfg.Origin != "" && cfg.SessionTTL > 0 && cfg.LoginRateLimit > 0 && cfg.LoginRateWindow > 0
	for _, o := range options {
		o(m)
	}
	return m
}
func jsonReply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, code string) {
	jsonReply(w, status, map[string]string{"error": code})
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", errors.New("random generation failed")
	}
	return hex.EncodeToString(b), nil
}
func tokenFormat(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func (m *Manager) cookieName(pre bool) string {
	name := "mesh_session"
	if pre {
		name = "mesh_captcha"
	}
	if m.cfg.SecureCookie {
		return "__Host-" + name
	}
	return name
}
func (m *Manager) setCookie(w http.ResponseWriter, token string, maxAge int, pre bool) {
	http.SetCookie(w, &http.Cookie{Name: m.cookieName(pre), Value: token, Path: "/", HttpOnly: true, Secure: m.cfg.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func (m *Manager) getCookie(r *http.Request, pre bool) string {
	var found string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == m.cookieName(pre) {
			found = c.Value
			count++
		}
	}
	if count != 1 || !tokenFormat(found) {
		return ""
	}
	return found
}
func socketIP(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		return host
	}
	return r.RemoteAddr
}
func (m *Manager) originOK(r *http.Request) bool {
	values := r.Header.Values("Origin")
	return len(values) == 1 && values[0] == m.cfg.Origin
}
func (m *Manager) digest(answer string) []byte {
	h := hmac.New(sha256.New, m.key)
	h.Write([]byte(answer))
	return h.Sum(nil)
}
func (m *Manager) pruneLocked() {
	now := m.now()
	for k, s := range m.sessions {
		if !s.ExpiresAt.After(now) {
			delete(m.sessions, k)
		}
	}
	for k, c := range m.challenges {
		if !c.expires.After(now) {
			delete(m.challenges, k)
		}
	}
	for _, store := range []map[string]attempt{m.loginAttempts, m.captchaAttempts} {
		for k, a := range store {
			if !a.until.After(now) {
				delete(store, k)
			}
		}
	}
}
func (m *Manager) rateLocked(store map[string]attempt, ip string, limit int, window time.Duration) (bool, int) {
	a, ok := store[ip]
	if !ok {
		if len(store) >= 10000 {
			return false, 1
		}
		a.until = m.now().Add(window)
	}
	a.count++
	store[ip] = a
	if a.count <= limit {
		return true, 0
	}
	retry := int(a.until.Sub(m.now()).Seconds() + .999)
	if retry < 1 {
		retry = 1
	}
	return false, retry
}
func (m *Manager) sessionLocked(r *http.Request) (Session, bool) {
	token := m.getCookie(r, false)
	s, ok := m.sessions[token]
	if !ok || !s.ExpiresAt.After(m.now()) {
		return Session{}, false
	}
	s.token = token
	return s, true
}

// Require protects every business endpoint. Unsafe HTTP methods additionally require exact Origin and CSRF.
func (m *Manager) Require(w http.ResponseWriter, r *http.Request) (Session, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if !m.valid {
		fail(w, 503, "auth_unavailable")
		return Session{}, false
	}
	m.mu.Lock()
	m.pruneLocked()
	s, ok := m.sessionLocked(r)
	m.mu.Unlock()
	if !ok {
		fail(w, 401, "unauthenticated")
		return Session{}, false
	}
	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
		csrf := r.Header.Get("X-CSRF-Token")
		if !m.originOK(r) || !tokenFormat(csrf) || subtle.ConstantTimeCompare([]byte(csrf), []byte(s.CSRFToken)) != 1 {
			fail(w, 403, "invalid_csrf")
			return Session{}, false
		}
	}
	return s, true
}
func (m *Manager) publicSession(s Session) any {
	return struct {
		Authenticated bool              `json:"authenticated"`
		Username      string            `json:"username"`
		User          map[string]string `json:"user"`
		CSRFToken     string            `json:"csrfToken"`
		ExpiresAt     time.Time         `json:"expiresAt"`
	}{true, s.Username, map[string]string{"username": s.Username}, s.CSRFToken, s.ExpiresAt}
}

// Handle returns false for non-auth paths, leaving all other routing to the caller.
func (m *Manager) Handle(w http.ResponseWriter, r *http.Request) bool {
	p := r.URL.Path
	if p != "/api/auth/captcha" && p != "/api/auth/login" && p != "/api/auth/session" && p != "/api/auth/logout" {
		return false
	}
	if !m.valid {
		fail(w, 503, "auth_unavailable")
		return true
	}
	switch p {
	case "/api/auth/captcha":
		if r.Method != "POST" {
			fail(w, 405, "method_not_allowed")
		} else {
			m.issueCaptcha(w, r)
		}
	case "/api/auth/login":
		if r.Method != "POST" {
			fail(w, 405, "method_not_allowed")
		} else {
			m.login(w, r)
		}
	case "/api/auth/session":
		if r.Method != "GET" {
			fail(w, 405, "method_not_allowed")
		} else if s, ok := m.Require(w, r); ok {
			jsonReply(w, 200, m.publicSession(s))
		}
	case "/api/auth/logout":
		if r.Method != "POST" {
			fail(w, 405, "method_not_allowed")
		} else if s, ok := m.Require(w, r); ok {
			m.mu.Lock()
			delete(m.sessions, s.token)
			m.mu.Unlock()
			m.setCookie(w, "", -1, false)
			jsonReply(w, 200, map[string]bool{"authenticated": false})
		}
	}
	return true
}
func (m *Manager) issueCaptcha(w http.ResponseWriter, r *http.Request) {
	if !m.originOK(r) {
		fail(w, 403, "invalid_origin")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	ok, retry := m.rateLocked(m.captchaAttempts, socketIP(r), 20, time.Minute)
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		fail(w, 429, "rate_limited")
		return
	}
	binding := m.getCookie(r, true)
	if binding != "" {
		delete(m.challenges, binding)
	}
	if len(m.challenges) >= 1000 {
		fail(w, 503, "captcha_capacity")
		return
	}
	answer, img, e := m.factory()
	if e != nil || len(answer) != 4 || len(img) > 150000 || len(img) < 6 || string(img[:6]) != "GIF89a" {
		fail(w, 500, "internal_error")
		return
	}
	for _, c := range answer {
		if c < 'A' || c > 'Z' {
			fail(w, 500, "internal_error")
			return
		}
	}
	if binding == "" {
		binding, e = randomToken()
		if e != nil {
			fail(w, 500, "internal_error")
			return
		}
	}
	id, e := randomToken()
	if e != nil {
		fail(w, 500, "internal_error")
		return
	}
	expiry := m.now().Add(120 * time.Second)
	m.challenges[binding] = challenge{id, m.digest(answer), expiry}
	m.setCookie(w, binding, 120, true)
	jsonReply(w, 200, struct {
		ID        string    `json:"captchaId"`
		Image     string    `json:"image"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{id, "data:image/gif;base64," + base64.StdEncoding.EncodeToString(img), expiry})
}
func (m *Manager) login(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.pruneLocked()
	binding := m.getCookie(r, true)
	owned, has := m.challenges[binding]
	delete(m.challenges, binding)
	ok, retry := m.rateLocked(m.loginAttempts, socketIP(r), m.cfg.LoginRateLimit, m.cfg.LoginRateWindow)
	m.mu.Unlock()
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		fail(w, 429, "rate_limited")
		return
	}
	if !m.originOK(r) {
		fail(w, 403, "invalid_origin")
		return
	}
	mt, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || mt != "application/json" {
		fail(w, 415, "invalid_request")
		return
	}
	var b struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		CaptchaID string `json:"captchaId"`
		Captcha   string `json:"captcha"`
	}
	data, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
	if e != nil {
		var max *http.MaxBytesError
		if errors.As(e, &max) {
			fail(w, 413, "invalid_request")
		} else {
			fail(w, 400, "invalid_request")
		}
		return
	}
	if e = json.Unmarshal(data, &b); e != nil {
		fail(w, 400, "invalid_request")
		return
	}
	answer := strings.ToUpper(strings.TrimSpace(b.Captcha))
	expected := owned.digest
	if len(expected) != 32 {
		expected = make([]byte, 32)
	}
	same := subtle.ConstantTimeCompare(m.digest(answer), expected) == 1
	if !has || !owned.expires.After(m.now()) || b.CaptchaID != owned.id || len(answer) != 4 || !same {
		fail(w, 400, "invalid_captcha")
		return
	}
	if b.Username == "" || len(b.Username) > 128 || b.Password == "" || len(b.Password) > 1024 {
		fail(w, 400, "invalid_request")
		return
	}
	select {
	case m.scryptSlots <- struct{}{}:
	default:
		fail(w, 429, "rate_limited")
		return
	}
	valid, e := VerifyPassword(b.Password, m.cfg.PasswordHash)
	<-m.scryptSlots
	if e != nil {
		fail(w, 500, "internal_error")
		return
	}
	if !valid || b.Username != m.cfg.Username {
		fail(w, 401, "invalid_credentials")
		return
	}
	token, e := randomToken()
	if e != nil {
		fail(w, 500, "internal_error")
		return
	}
	csrf, e := randomToken()
	if e != nil {
		fail(w, 500, "internal_error")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	if len(m.sessions) >= 10000 {
		fail(w, 503, "session_capacity")
		return
	}
	old := m.getCookie(r, false)
	delete(m.sessions, old)
	s := Session{Username: m.cfg.Username, CSRFToken: csrf, ExpiresAt: m.now().Add(m.cfg.SessionTTL), token: token}
	m.sessions[token] = s
	m.setCookie(w, token, int(m.cfg.SessionTTL/time.Second), false)
	jsonReply(w, 200, m.publicSession(s))
}
