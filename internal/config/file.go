package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileConfig is a strict, operator-owned YAML configuration, never served to the browser.
type FileConfig struct {
	Environment string `yaml:"environment"`
	Demo        *bool  `yaml:"demo"`
	Server      struct {
		Host         string `yaml:"host"`
		Port         *int   `yaml:"port"`
		Origin       string `yaml:"origin"`
		SecureCookie *bool  `yaml:"secure_cookie"`
	} `yaml:"server"`
	Auth struct {
		Username         string `yaml:"username"`
		PasswordHash     string `yaml:"password_hash"`
		PasswordHashFile string `yaml:"password_hash_file"`
		SessionTTL       *int   `yaml:"session_ttl_seconds"`
		LoginRateLimit   *int   `yaml:"login_rate_limit"`
		LoginRateWindow  *int   `yaml:"login_rate_window_seconds"`
	} `yaml:"auth"`
	Headscale struct {
		URL        string `yaml:"url"`
		PublicURL  string `yaml:"public_url"`
		APIKey     string `yaml:"api_key"`
		APIKeyFile string `yaml:"api_key_file"`
		PolicyMode string `yaml:"policy_mode"`
	} `yaml:"headscale"`
	Configuration struct {
		Path            string `yaml:"path"`
		WriteEnabled    *bool  `yaml:"write_enabled"`
		TestExecutable  string `yaml:"test_executable"`
		ApplyExecutable string `yaml:"apply_executable"`
	} `yaml:"configuration"`
}

// LoadFile validates the merged YAML and process environment using the same security rules as Load.
// Relative file paths resolve from the YAML directory. An env secret or _FILE overrides both YAML variants.
func LoadFile(filename string, lookup func(string) (string, bool)) (Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return Config{}, errors.New("cannot open console configuration file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return Config{}, errors.New("invalid console configuration file")
	}
	var f FileConfig
	decoder := yaml.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.KnownFields(true)
	if decoder.Decode(&f) != nil {
		return Config{}, errors.New("invalid console configuration YAML or unknown field")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return Config{}, errors.New("console configuration must contain one YAML document")
	}
	abs, err := filepath.Abs(filename)
	if err != nil {
		return Config{}, errors.New("invalid configuration path")
	}
	base := filepath.Dir(abs)
	resolve := func(p string) string {
		if p != "" && !filepath.IsAbs(p) {
			return filepath.Join(base, p)
		}
		return p
	}
	values := map[string]string{
		"APP_ENV": f.Environment, "HOST": f.Server.Host, "CONSOLE_ORIGIN": f.Server.Origin,
		"CONSOLE_USERNAME": f.Auth.Username, "CONSOLE_PASSWORD_HASH": f.Auth.PasswordHash, "CONSOLE_PASSWORD_HASH_FILE": resolve(f.Auth.PasswordHashFile),
		"HEADSCALE_URL": f.Headscale.URL, "HEADSCALE_PUBLIC_URL": f.Headscale.PublicURL, "HEADSCALE_API_KEY": f.Headscale.APIKey, "HEADSCALE_API_KEY_FILE": resolve(f.Headscale.APIKeyFile), "HEADSCALE_POLICY_MODE": f.Headscale.PolicyMode,
		"HEADSCALE_CONFIG_PATH": resolve(f.Configuration.Path), "CONFIG_TEST_EXECUTABLE": resolve(f.Configuration.TestExecutable), "APPLY_EXECUTABLE": resolve(f.Configuration.ApplyExecutable),
	}
	boolValue := func(k string, p *bool) {
		if p != nil {
			values[k] = strconv.FormatBool(*p)
		}
	}
	intValue := func(k string, p *int) {
		if p != nil {
			values[k] = strconv.Itoa(*p)
		}
	}
	boolValue("DEMO_MODE", f.Demo)
	boolValue("COOKIE_SECURE", f.Server.SecureCookie)
	boolValue("CONFIG_WRITE_ENABLED", f.Configuration.WriteEnabled)
	intValue("PORT", f.Server.Port)
	intValue("SESSION_TTL_SECONDS", f.Auth.SessionTTL)
	intValue("LOGIN_RATE_LIMIT", f.Auth.LoginRateLimit)
	intValue("LOGIN_RATE_WINDOW_SECONDS", f.Auth.LoginRateWindow)
	for _, key := range []string{"CONSOLE_USERNAME", "CONSOLE_PASSWORD_HASH", "HEADSCALE_URL", "HEADSCALE_PUBLIC_URL", "HEADSCALE_API_KEY"} {
		_, direct := lookup(key)
		_, fromFile := lookup(key + "_FILE")
		if direct || fromFile {
			delete(values, key)
			delete(values, key+"_FILE")
		}
	}
	return Load(func(key string) string {
		if v, ok := lookup(key); ok {
			return v
		}
		return strings.TrimSpace(values[key])
	})
}
