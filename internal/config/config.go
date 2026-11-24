// Package config loads validated server-only deployment settings.
package config

import (
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host                 string
	Port                 int
	Origin               string
	SecureCookie         bool
	Username             string
	PasswordHash         string
	HeadscaleURL         string
	HeadscalePublicURL   string
	HeadscaleAPIKey      string
	PolicyMode           string
	Demo                 bool
	SessionTTL           time.Duration
	LoginRateLimit       int
	LoginRateWindow      time.Duration
	DistDir              string
	ConfigPath           string
	ConfigWriteEnabled   bool
	ConfigTestExecutable string
	ApplyExecutable      string
	DataDir              string
}

func ValidatePasswordHash(value string) error {
	p := strings.Split(value, "$")
	if len(p) != 6 || p[0] != "scrypt" || p[1] != "16384" || p[2] != "8" || p[3] != "1" {
		return errors.New("invalid password hash format")
	}
	for i, n := range []int{16, 64} {
		b, e := base64.StdEncoding.Strict().DecodeString(p[i+4])
		if e != nil || len(b) != n || base64.StdEncoding.EncodeToString(b) != p[i+4] {
			return errors.New("invalid password hash encoding")
		}
	}
	return nil
}
func LoadEnv() (Config, error) { return Load(os.Getenv) }
func Load(getenv func(string) string) (Config, error) {
	c := Config{Host: "127.0.0.1", Port: 3080, SessionTTL: time.Hour, LoginRateLimit: 10, LoginRateWindow: 900 * time.Second, DistDir: "internal/webassets/static/dist", DataDir: "data", PolicyMode: "unknown"}
	secret := func(name string) (string, error) {
		v, f := getenv(name), getenv(name+"_FILE")
		if v != "" && f != "" {
			return "", errors.New("configure only " + name + " or " + name + "_FILE")
		}
		if f != "" {
			b, e := os.ReadFile(f)
			if e != nil {
				return "", errors.New("cannot read " + name + "_FILE")
			}
			v = string(b)
		}
		return strings.TrimSpace(v), nil
	}
	var err error
	for name, target := range map[string]*string{"CONSOLE_USERNAME": &c.Username, "CONSOLE_PASSWORD_HASH": &c.PasswordHash, "HEADSCALE_URL": &c.HeadscaleURL, "HEADSCALE_PUBLIC_URL": &c.HeadscalePublicURL, "HEADSCALE_API_KEY": &c.HeadscaleAPIKey} {
		*target, err = secret(name)
		if err != nil {
			return c, err
		}
	}
	if c.Username == "" || len(c.Username) > 128 || c.PasswordHash == "" {
		return c, errors.New("console username and password hash required including demo")
	}
	if err = ValidatePasswordHash(c.PasswordHash); err != nil {
		return c, err
	}
	boolean := func(name string) (bool, error) {
		v := getenv(name)
		if v != "" && v != "true" && v != "false" {
			return false, errors.New("invalid " + name)
		}
		return v == "true", nil
	}
	if c.Demo, err = boolean("DEMO_MODE"); err != nil {
		return c, err
	}
	if c.SecureCookie, err = boolean("COOKIE_SECURE"); err != nil {
		return c, err
	}
	if c.ConfigWriteEnabled, err = boolean("CONFIG_WRITE_ENABLED"); err != nil {
		return c, err
	}
	if v := getenv("HOST"); v != "" {
		c.Host = v
	}
	number := func(name string, def, min, max int) (int, error) {
		s := getenv(name)
		if s == "" {
			return def, nil
		}
		for _, r := range s {
			if r < '0' || r > '9' {
				return 0, errors.New("invalid " + name)
			}
		}
		n, e := strconv.Atoi(s)
		if e != nil || n < min || n > max {
			return 0, errors.New("invalid " + name)
		}
		return n, nil
	}
	if c.Port, err = number("PORT", 3080, 0, 65535); err != nil {
		return c, err
	}
	ttl, e := number("SESSION_TTL_SECONDS", 3600, 60, 86400)
	if e != nil {
		return c, e
	}
	c.SessionTTL = time.Duration(ttl) * time.Second
	if c.LoginRateLimit, err = number("LOGIN_RATE_LIMIT", 10, 1, 100); err != nil {
		return c, err
	}
	window, e := number("LOGIN_RATE_WINDOW_SECONDS", 900, 1, 3600)
	if e != nil {
		return c, e
	}
	c.LoginRateWindow = time.Duration(window) * time.Second
	validateURL := func(value, name string) error {
		if value == "" {
			return nil
		}
		u, e := url.Parse(value)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(value, "\r\n") {
			return errors.New("invalid " + name)
		}
		return nil
	}
	if err = validateURL(c.HeadscaleURL, "HEADSCALE_URL"); err != nil {
		return c, err
	}
	if err = validateURL(c.HeadscalePublicURL, "HEADSCALE_PUBLIC_URL"); err != nil {
		return c, err
	}
	if !c.Demo && (c.HeadscaleURL == "" || c.HeadscalePublicURL == "" || c.HeadscaleAPIKey == "") {
		return c, errors.New("live requires backend URL public URL and API key")
	}
	if strings.ContainsAny(c.HeadscaleAPIKey, "\r\n") {
		return c, errors.New("invalid HEADSCALE_API_KEY")
	}
	c.Origin = getenv("CONSOLE_ORIGIN")
	loopback := c.Host == "localhost"
	if ip := net.ParseIP(c.Host); ip != nil {
		loopback = ip.IsLoopback()
	}
	if c.Origin == "" {
		if !loopback || c.Port == 0 {
			return c, errors.New("explicit CONSOLE_ORIGIN required")
		}
		c.Origin = "http://" + net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	}
	u, e := url.Parse(c.Origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Scheme+"://"+u.Host != c.Origin {
		return c, errors.New("invalid CONSOLE_ORIGIN")
	}
	appEnv := getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "production"
	}
	if appEnv != "production" && appEnv != "development" {
		return c, errors.New("invalid APP_ENV")
	}
	if !c.SecureCookie && (appEnv != "development" || !loopback) {
		return c, errors.New("secure cookie required outside local development")
	}
	if c.SecureCookie && u.Scheme != "https" {
		return c, errors.New("secure cookie requires HTTPS origin")
	}
	if !c.SecureCookie {
		oh := u.Hostname()
		ip := net.ParseIP(oh)
		if oh != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return c, errors.New("development HTTP origin must be loopback")
		}
	}
	if v := getenv("HEADSCALE_POLICY_MODE"); v != "" {
		c.PolicyMode = v
	}
	if c.PolicyMode != "file" && c.PolicyMode != "database" && c.PolicyMode != "unknown" {
		return c, errors.New("invalid HEADSCALE_POLICY_MODE")
	}
	for name, target := range map[string]*string{"DIST_DIR": &c.DistDir, "DATA_DIR": &c.DataDir, "HEADSCALE_CONFIG_PATH": &c.ConfigPath, "CONFIG_TEST_EXECUTABLE": &c.ConfigTestExecutable, "APPLY_EXECUTABLE": &c.ApplyExecutable} {
		if v := getenv(name); v != "" {
			*target = v
		}
	}
	if c.ConfigWriteEnabled {
		if !filepath.IsAbs(c.ConfigPath) || !filepath.IsAbs(c.ConfigTestExecutable) || !filepath.IsAbs(c.ApplyExecutable) {
			return c, errors.New("config writes require absolute config and trusted executable paths")
		}
		for _, p := range []string{c.ConfigTestExecutable, c.ApplyExecutable} {
			i, e := os.Stat(p)
			if e != nil || !i.Mode().IsRegular() {
				return c, errors.New("trusted config executable unavailable")
			}
		}
	}
	return c, nil
}
