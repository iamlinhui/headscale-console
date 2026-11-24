package configstore

import (
	"encoding/json"
	"errors"
	"net/http"
)

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, err error) {
	code := err.Error()
	status := 400
	if errors.Is(err, ErrReadOnly) {
		status = 403
	} else if errors.Is(err, ErrConflict) {
		status = 409
	} else if errors.Is(err, ErrUnavailable) {
		status = 503
	} else if errors.Is(err, ErrApply) {
		status = 502
	}
	respond(w, status, map[string]string{"error": code, "message": "配置操作未完成；请检查配置权限、校验与服务应用状态。"})
}

// ServeHTTP must be called only after the enclosing server's authentication/CSRF gate.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request, owner string) {
	switch {
	case r.URL.Path == "/api/config/dns" && r.Method == http.MethodGet:
		if s.opts.Path == "" {
			respond(w, 503, map[string]string{"error": "configuration_not_configured", "message": "DNS 不由 Headscale REST API 提供；请将当前服务端配置只读挂载到管理端，并设置 configuration.path。"})
			return
		}
		snap, err := s.Get()
		if err != nil {
			failure(w, err)
			return
		}
		respond(w, 200, snap)
	case r.URL.Path == "/api/config/dns/preview" && r.Method == http.MethodPost:
		var req struct {
			DNS      DNS    `json:"dns"`
			Revision string `json:"revision"`
		}
		if !decode(w, r, &req) {
			return
		}
		p, err := s.Preview(r.Context(), owner, req.Revision, req.DNS)
		if err != nil {
			failure(w, err)
			return
		}
		respond(w, 200, p)
	case r.URL.Path == "/api/config/dns/apply" && r.Method == http.MethodPost:
		var req struct {
			Token     string `json:"token"`
			Confirmed bool   `json:"confirmed"`
		}
		if !decode(w, r, &req) {
			return
		}
		if !req.Confirmed {
			respond(w, 400, map[string]string{"error": "confirmation_required"})
			return
		}
		snap, err := s.Apply(r.Context(), owner, req.Token)
		if err != nil {
			failure(w, err)
			return
		}
		respond(w, 200, snap)
	default:
		respond(w, 404, map[string]string{"error": "not_found"})
	}
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		respond(w, 415, map[string]string{"error": "invalid_request"})
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		respond(w, 400, map[string]string{"error": "invalid_request"})
		return false
	}
	return true
}
