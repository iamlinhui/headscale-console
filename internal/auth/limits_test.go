package auth

import (
	"net/http"
	"testing"
	"time"
)

func TestBoundedCapacitiesAndPasswordConcurrency(t *testing.T) {
	f := setup(t)
	for i := 0; i < 1000; i++ {
		f.m.challenges[string(rune(i))] = challenge{expires: f.now.Add(time.Minute)}
	}
	if f.request("/api/auth/captcha", "POST", nil, nil, nil).Code != 503 {
		t.Fatal("captcha capacity not bounded")
	}
	f = setup(t)
	for i := 0; i < 10000; i++ {
		f.m.loginAttempts[string(rune(i))] = attempt{until: f.now.Add(time.Minute)}
	}
	if f.login("", nil, "", "bad").Code != 429 {
		t.Fatal("rate map unbounded")
	}
	f = setup(t)
	id, c := f.claim(nil)
	for i := 0; i < 4; i++ {
		f.m.scryptSlots <- struct{}{}
	}
	if f.login(id, c, "TEST", "NodeCompatible-password!").Code != 429 {
		t.Fatal("password concurrency not bounded")
	}
	for i := 0; i < 4; i++ {
		<-f.m.scryptSlots
	}
	if f.login(id, c, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("busy attempt did not consume captcha")
	}
	f = setup(t)
	id, c = f.claim(nil)
	for i := 0; i < 10000; i++ {
		f.m.sessions[string(rune(i))] = Session{ExpiresAt: f.now.Add(time.Hour)}
	}
	if f.login(id, c, "TEST", "NodeCompatible-password!").Code != 503 {
		t.Fatal("session capacity not bounded")
	}
}
func TestDuplicateCookiesAndMalformedAttemptsConsume(t *testing.T) {
	f := setup(t)
	id, c := f.claim(nil)
	duplicate := append(c, &http.Cookie{Name: "mesh_captcha", Value: c[0].Value})
	if f.login(id, duplicate, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("duplicate cookies accepted")
	}
	id, c = f.claim(c)
	w := f.request("/api/auth/login", "POST", map[string]any{"username": 7, "password": "test", "captchaId": id, "captcha": "TEST"}, c, nil)
	if w.Code != 400 {
		t.Fatal("malformed body accepted")
	}
	if f.login(id, c, "TEST", "NodeCompatible-password!").Code != 400 {
		t.Fatal("malformed attempt reuse")
	}
}
