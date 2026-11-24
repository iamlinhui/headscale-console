package tests

import (
	"gopkg.in/yaml.v3"
	"meshconsole/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFinalHeadscaleDeploymentIsConsistent(t *testing.T) {
	root := filepath.Join("..", "docs", "headscale")
	read := func(p string) map[string]any {
		b, e := os.ReadFile(filepath.Join(root, p))
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if yaml.Unmarshal(b, &v) != nil {
			t.Fatal("invalid yaml", p)
		}
		return v
	}
	hs := read("examples/headscale.config.example.yaml")
	if hs["policy"].(map[string]any)["mode"] != "database" {
		t.Fatal("policy mode mismatch")
	}
	if hs["server_url"] != "https://headscale.example.com" {
		t.Fatal("public URL changed")
	}
	compose := read("docker-compose.yml")
	services := compose["services"].(map[string]any)
	if services["headscale"].(map[string]any)["image"] != "headscale/headscale:0.29.7" {
		t.Fatal("version not pinned")
	}
	if !strings.Contains(services["headscale-console"].(map[string]any)["image"].(string), "promptness/headscale-console") {
		t.Fatal("wrong repository")
	}
	web := read("examples/console.docker.example.yaml")
	if web["headscale"].(map[string]any)["policy_mode"] != "database" || web["server"].(map[string]any)["origin"] != "https://headscale.example.com" {
		t.Fatal("console mismatch")
	}
	if web["configuration"].(map[string]any)["write_enabled"] != false {
		t.Fatal("DNS write unexpectedly enabled")
	}
	b, _ := os.ReadFile(filepath.Join(root, "examples", "Caddyfile.example"))
	for _, needle := range []string{"handle_path /admin/*", "redir @console /admin/ 308", "127.0.0.1:9983", "127.0.0.1:9981"} {
		if !strings.Contains(string(b), needle) {
			t.Fatal("caddy mismatch", needle)
		}
	}
}
func TestFinalConsoleTemplateValidatesWithFixtureSecrets(t *testing.T) {
	b, e := os.ReadFile(filepath.Join("..", "docs", "headscale", "examples", "console.docker.example.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	var f config.FileConfig
	if yaml.Unmarshal(b, &f) != nil {
		t.Fatal("invalid console YAML")
	}
	tmp := t.TempDir()
	hash := "scrypt$16384$8$1$AAAAAAAAAAAAAAAAAAAAAA==$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
	hashFile := filepath.Join(tmp, "hash")
	keyFile := filepath.Join(tmp, "key")
	os.WriteFile(hashFile, []byte(hash), 0600)
	os.WriteFile(keyFile, []byte("TEST_MANAGEMENT_KEY"), 0600)
	env := map[string]string{"CONSOLE_PASSWORD_HASH_FILE": hashFile, "HEADSCALE_API_KEY_FILE": keyFile}
	c, e := config.LoadFile(filepath.Join("..", "docs", "headscale", "examples", "console.docker.example.yaml"), func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if e != nil {
		t.Fatal(e)
	}
	if c.Demo || !c.SecureCookie || c.PolicyMode != "database" || c.HeadscaleURL != "http://headscale:9981" {
		t.Fatal("invalid final console config")
	}
}
