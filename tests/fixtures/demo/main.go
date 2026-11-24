// Only compiled by E2E tests. Fixed credentials/captcha never enter production cmd/console.
package main

import (
	"io/fs"
	"log"
	"meshconsole/internal/auth"
	"meshconsole/internal/captcha"
	"meshconsole/internal/config"
	"meshconsole/internal/webassets"
	"meshconsole/internal/webserver"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func main() {
	hash, err := auth.HashPassword("E2E-fixture-only!")
	if err != nil {
		log.Fatal(err)
	}
	values := map[string]string{"APP_ENV": "development", "HOST": "127.0.0.1", "PORT": "3081", "CONSOLE_ORIGIN": "http://127.0.0.1:3081", "COOKIE_SECURE": "false", "DEMO_MODE": "true", "CONSOLE_USERNAME": "ui-test", "CONSOLE_PASSWORD_HASH": hash, "HEADSCALE_URL": "https://headscale.test.internal", "HEADSCALE_PUBLIC_URL": "https://headscale.test.internal", "HEADSCALE_API_KEY": "E2E_SERVER_SECRET_NEVER_SHIPPED"}
	cfg, err := config.Load(func(k string) string { return values[k] })
	if err != nil {
		log.Fatal(err)
	}
	frontend, _ := fs.Sub(webassets.Frontend, "static/dist")
	handler, err := webserver.New(cfg, frontend, auth.WithCaptchaFactory(func() (string, []byte, error) { return captcha.Generate("TEST") }))
	if err != nil {
		log.Fatal(err)
	}
	adminCfg := cfg
	adminCfg.Origin = "http://127.0.0.1:3083"
	adminHandler, err := webserver.New(adminCfg, frontend, auth.WithCaptchaFactory(func() (string, []byte, error) { return captcha.Generate("TEST") }))
	if err != nil {
		log.Fatal(err)
	}
	go func() { log.Fatal(http.ListenAndServe("127.0.0.1:3084", adminHandler)) }()
	proxyURL, _ := url.Parse("http://127.0.0.1:3084")
	proxy := httputil.NewSingleHostReverseProxy(proxyURL)
	go func() {
		log.Fatal(http.ListenAndServe("127.0.0.1:3083", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/admin" {
				http.Redirect(w, r, "/admin/", 308)
				return
			}
			if !strings.HasPrefix(r.URL.Path, "/admin/") {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/admin")
			proxy.ServeHTTP(w, r)
		})))
	}()
	log.Fatal(http.ListenAndServe("127.0.0.1:3081", handler))
}
