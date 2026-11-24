package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fileTestHash = "scrypt$16384$8$1$AAAAAAAAAAAAAAAAAAAAAA==$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="

func fileFixture(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func baseYAML() string {
	return "environment: development\ndemo: true\nserver:\n  host: 127.0.0.1\n  port: 3080\n  origin: http://127.0.0.1:5173\n  secure_cookie: false\nauth:\n  username: admin\n  password_hash: '" + fileTestHash + "'\n"
}
func TestLoadFileAndRelativeSecret(t *testing.T) {
	p := fileFixture(t, strings.Replace(baseYAML(), "password_hash: '"+fileTestHash+"'", "password_hash_file: password.hash", 1))
	_ = os.WriteFile(filepath.Join(filepath.Dir(p), "password.hash"), []byte(fileTestHash), 0600)
	c, e := LoadFile(p, func(string) (string, bool) { return "", false })
	if e != nil || c.Username != "admin" || c.Origin != "http://127.0.0.1:5173" || c.PasswordHash != fileTestHash {
		t.Fatal(c, e)
	}
}
func TestEnvironmentOverridesSecretPair(t *testing.T) {
	p := fileFixture(t, baseYAML())
	secret := filepath.Join(filepath.Dir(p), "override.hash")
	_ = os.WriteFile(secret, []byte(fileTestHash), 0600)
	env := map[string]string{"CONSOLE_USERNAME": "operator", "CONSOLE_PASSWORD_HASH_FILE": secret, "PORT": "3090"}
	c, e := LoadFile(p, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if e != nil || c.Username != "operator" || c.Port != 3090 {
		t.Fatal(c, e)
	}
}
func TestFileFailsClosed(t *testing.T) {
	for _, suffix := range []string{"unknown_field: secret_value\n", "---\ndemo: true\n", "demo: false\n"} {
		p := fileFixture(t, baseYAML()+suffix)
		_, e := LoadFile(p, func(string) (string, bool) { return "", false })
		if e == nil || strings.Contains(e.Error(), "secret_value") {
			t.Fatal(e)
		}
	}
	p := fileFixture(t, strings.Replace(baseYAML(), fileTestHash, "", 1))
	if _, e := LoadFile(p, func(string) (string, bool) { return "", false }); e == nil {
		t.Fatal("empty hash accepted")
	}
	if _, e := LoadFile(filepath.Join(t.TempDir(), "missing.yaml"), func(string) (string, bool) { return "", false }); e == nil {
		t.Fatal("missing file accepted")
	}
}
