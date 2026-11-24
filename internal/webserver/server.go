// Package webserver wires the authentication gate to the business handlers.
package webserver

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"meshconsole/internal/auth"
	"meshconsole/internal/config"
	"meshconsole/internal/configstore"
	"meshconsole/internal/headscale"
)

type Server struct {
	cfg      config.Config
	auth     *auth.Manager
	upstream *headscale.Client
	store    *configstore.Store
	frontend fs.FS
}

func New(cfg config.Config, frontend fs.FS, authOptions ...auth.Option) (http.Handler, error) {
	a := auth.New(cfg, authOptions...)
	var client *headscale.Client
	var err error
	if !cfg.Demo {
		client, err = headscale.New(cfg.HeadscaleURL, cfg.HeadscaleAPIKey, cfg.HeadscalePublicURL, cfg.PolicyMode)
		if err != nil {
			return nil, err
		}
	}
	if cfg.Demo && cfg.ConfigWriteEnabled {
		return nil, fmt.Errorf("configuration writes require a live Headscale health probe")
	}
	storeOptions := configstore.Options{Path: cfg.ConfigPath, WriteEnabled: cfg.ConfigWriteEnabled, ConfigTestExecutable: cfg.ConfigTestExecutable, ApplyExecutable: cfg.ApplyExecutable}
	if client != nil {
		storeOptions.Verify = client.Verify
	}
	store, err := configstore.New(storeOptions)
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, auth: a, upstream: client, store: store, frontend: frontend}, nil
}
func output(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	if s.cfg.SecureCookie {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
	p := r.URL.Path
	if strings.ContainsAny(p, "\x00\\") {
		output(w, 400, map[string]string{"error": "invalid_path"})
		return
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || strings.HasPrefix(part, ".") {
			output(w, 400, map[string]string{"error": "invalid_path"})
			return
		}
	}
	if p == "/api" || strings.HasPrefix(p, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
		if p == "/api/health" && r.Method == http.MethodGet {
			output(w, 200, map[string]string{"status": "ok"})
			return
		}
		if s.auth.Handle(w, r) {
			return
		}
		session, ok := s.auth.Require(w, r)
		if !ok {
			return
		}
		if strings.HasPrefix(p, "/api/config/") {
			s.store.ServeHTTP(w, r, session.Username)
			return
		}
		if s.cfg.Demo {
			if p == "/api/connection" && r.Method == http.MethodGet {
				output(w, 200, map[string]any{"url": s.cfg.HeadscaleURL, "publicUrl": s.cfg.HeadscalePublicURL, "mode": "demo", "apiKeyConfigured": s.cfg.HeadscaleAPIKey != "", "configured": s.cfg.HeadscaleURL != "" && s.cfg.HeadscaleAPIKey != "", "connected": false, "version": "", "policyMode": s.cfg.PolicyMode, "capabilities": map[string]bool{"read": true, "write": true, "keys": true, "policyCheck": true, "policyWrite": true}})
				return
			}
			output(w, 404, map[string]string{"error": "demo_endpoint_unavailable"})
			return
		}
		s.upstream.Handler().ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		output(w, 405, map[string]string{"error": "method_not_allowed"})
		return
	}
	if s.frontend == nil {
		output(w, 503, map[string]string{"error": "frontend_not_built"})
		return
	}
	name := strings.TrimPrefix(path.Clean(p), "/")
	if name == "" {
		name = "index.html"
	}
	b, err := fs.ReadFile(s.frontend, name)
	if err != nil {
		if path.Ext(name) != "" {
			output(w, 404, map[string]string{"error": "not_found"})
			return
		}
		name = "index.html"
		b, err = fs.ReadFile(s.frontend, name)
	}
	if err != nil {
		output(w, 404, map[string]string{"error": "frontend_not_built"})
		return
	}
	typ := mime.TypeByExtension(path.Ext(name))
	if typ == "" {
		typ = "application/octet-stream"
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method != http.MethodHead {
		_, _ = w.Write(b)
	}
}
