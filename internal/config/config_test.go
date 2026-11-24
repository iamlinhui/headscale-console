package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testHash = "scrypt$16384$8$1$BwcHBwcHBwcHBwcHBwcHBw==$ITVJINRlwVHQkq7R/OIygorvwc5zydc+Y/lRZx55NtPI27cqgUNidOLB4IkS42i3xJ92AU813Blyn0XCW8E7mA=="

func env(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}
func basic() map[string]string {
	return map[string]string{"APP_ENV": "development", "DEMO_MODE": "true", "CONSOLE_USERNAME": "admin", "CONSOLE_PASSWORD_HASH": testHash}
}
func TestDefaultsAndProduction(t *testing.T) {
	c, e := Load(env(basic()))
	if e != nil {
		t.Fatal(e)
	}
	if c.Port != 3080 || c.Host != "127.0.0.1" || c.Origin != "http://127.0.0.1:3080" || c.LoginRateLimit != 10 {
		t.Fatal("defaults")
	}
	v := basic()
	delete(v, "APP_ENV")
	if _, e = Load(env(v)); e == nil {
		t.Fatal("production insecure accepted")
	}
	v["COOKIE_SECURE"] = "true"
	v["CONSOLE_ORIGIN"] = "https://console.example.com"
	if _, e = Load(env(v)); e != nil {
		t.Fatal(e)
	}
}
func TestSecretsAndURLs(t *testing.T) {
	v := basic()
	dir := t.TempDir()
	p := filepath.Join(dir, "private.hash")
	if e := os.WriteFile(p, []byte(testHash+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	delete(v, "CONSOLE_PASSWORD_HASH")
	v["CONSOLE_PASSWORD_HASH_FILE"] = p
	if _, e := Load(env(v)); e != nil {
		t.Fatal(e)
	}
	v["CONSOLE_PASSWORD_HASH"] = testHash
	if _, e := Load(env(v)); e == nil {
		t.Fatal("ambiguous secret")
	}
	delete(v, "CONSOLE_PASSWORD_HASH")
	v["HEADSCALE_URL"] = "https://u:PRIVATE_SECRET@hs.example"
	if _, e := Load(env(v)); e == nil || strings.Contains(e.Error(), "PRIVATE_SECRET") {
		t.Fatal("invalid URL or secret error leakage")
	}
	v["HEADSCALE_URL"] = "http://headscale:9981"
	v["DEMO_MODE"] = "false"
	if _, e := Load(env(v)); e == nil {
		t.Fatal("live incomplete")
	}
	v["HEADSCALE_PUBLIC_URL"] = "https://hs.example.com"
	v["HEADSCALE_API_KEY"] = "private-test"
	if _, e := Load(env(v)); e != nil {
		t.Fatal(e)
	}
}
func TestOriginAndHashValidation(t *testing.T) {
	for k, value := range map[string]string{"HOST": "0.0.0.0", "PORT": "-1", "CONSOLE_ORIGIN": "https://console.example.com/path", "CONSOLE_PASSWORD_HASH": "bad", "APP_ENV": "test", "CONFIG_WRITE_ENABLED": "true"} {
		v := basic()
		v[k] = value
		if _, e := Load(env(v)); e == nil {
			t.Fatalf("accepted %s", k)
		}
	}
	v := basic()
	v["CONSOLE_ORIGIN"] = "http://evil.example"
	if _, e := Load(env(v)); e == nil {
		t.Fatal("development nonlocal origin")
	}
}
