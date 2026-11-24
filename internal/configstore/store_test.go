package configstore

import (
	"context"
	"errors"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = "# preserve comment\nserver_url: https://example.com\nnoise:\n  private_key_path: /secret/noise.key\ndns:\n  magic_dns: true\n  base_domain: hs.internal\n  override_local_dns: false\n  nameservers:\n    global: [1.1.1.1]\n    split:\n      private.internal: [10.1.1.1]\n"

func fixture(t *testing.T, opts Options) *Store {
	t.Helper()
	opts.Path = filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(opts.Path, []byte(sample), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestReadOnlyWhitelist(t *testing.T) {
	s := fixture(t, Options{})
	snap, e := s.Get()
	if e != nil || snap.DNS.Domain != "hs.internal" || snap.Writable {
		t.Fatal(snap, e)
	}
	if _, e = s.Preview(context.Background(), "admin", snap.Revision, snap.DNS); !errors.Is(e, ErrReadOnly) {
		t.Fatal(e)
	}
}
func TestPreviewConflictAndApply(t *testing.T) {
	validated, applied := 0, 0
	s := fixture(t, Options{WriteEnabled: true, Validate: func(_ context.Context, p string) error {
		validated++
		b, _ := os.ReadFile(p)
		if !strings.Contains(string(b), "mesh.internal") {
			return errors.New("bad")
		}
		return nil
	}, Apply: func(context.Context) error { applied++; return nil }})
	snap, _ := s.Get()
	target := snap.DNS
	target.Domain = "mesh.internal"
	target.Servers = []string{"9.9.9.9", "2606:4700:4700::1111"}
	if _, e := s.Preview(context.Background(), "admin", "bad", target); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	p, e := s.Preview(context.Background(), "admin", snap.Revision, target)
	if e != nil || validated != 1 {
		t.Fatal(e)
	}
	unchanged, _ := os.ReadFile(s.opts.Path)
	if string(unchanged) != sample {
		t.Fatal("preview wrote source")
	}
	out, e := s.Apply(context.Background(), "admin", p.Token)
	if e != nil || out.DNS.Domain != "mesh.internal" || applied != 1 {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(s.opts.Path)
	var m map[string]any
	if yaml.Unmarshal(b, &m) != nil {
		t.Fatal("invalid YAML")
	}
	if m["server_url"] != "https://example.com" || !strings.Contains(string(b), "preserve comment") || !strings.Contains(string(b), "private.internal") {
		t.Fatal("non-DNS config destroyed")
	}
	if _, e = s.Apply(context.Background(), "admin", p.Token); e == nil {
		t.Fatal("plan replay")
	}
}
func TestValidationFailureAndRollback(t *testing.T) {
	s := fixture(t, Options{WriteEnabled: true, Validate: func(context.Context, string) error { return nil }, Apply: func(context.Context) error { return errors.New("apply failure") }})
	snap, _ := s.Get()
	p, e := s.Preview(context.Background(), "admin", snap.Revision, snap.DNS)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(context.Background(), "admin", p.Token); e == nil {
		t.Fatal("failure ignored")
	}
	b, _ := os.ReadFile(s.opts.Path)
	if string(b) != sample {
		t.Fatal("rollback failed")
	}
	s.opts.Validate = func(context.Context, string) error { return errors.New("validation failure") }
	if _, e = s.Preview(context.Background(), "admin", snap.Revision, snap.DNS); e == nil {
		t.Fatal("validation ignored")
	}
}
func TestWrongOwnerAndConcurrentEdit(t *testing.T) {
	s := fixture(t, Options{WriteEnabled: true, Validate: func(context.Context, string) error { return nil }, Apply: func(context.Context) error { return nil }})
	snap, _ := s.Get()
	p, _ := s.Preview(context.Background(), "owner", snap.Revision, snap.DNS)
	if _, e := s.Apply(context.Background(), "other", p.Token); e == nil {
		t.Fatal("owner mismatch accepted")
	}
	p, _ = s.Preview(context.Background(), "owner", snap.Revision, snap.DNS)
	_ = os.WriteFile(s.opts.Path, []byte(sample+"# concurrent\n"), 0600)
	if _, e := s.Apply(context.Background(), "owner", p.Token); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
}
func TestLegacyNameserversBecomeGlobal(t *testing.T) {
	s := fixture(t, Options{WriteEnabled: true, Validate: func(context.Context, string) error { return nil }, Apply: func(context.Context) error { return nil }})
	_ = os.WriteFile(s.opts.Path, []byte("server_url: https://example.com\ndns:\n  magic_dns: true\n  base_domain: hs.internal\n  nameservers: []\n"), 0600)
	snap, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	target := snap.DNS
	target.Servers = []string{"9.9.9.9"}
	p, err := s.Preview(context.Background(), "admin", snap.Revision, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(context.Background(), "admin", p.Token); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.opts.Path)
	if !strings.Contains(string(b), "global:") || !strings.Contains(string(b), "9.9.9.9") {
		t.Fatal("nameservers.global not written")
	}
}
func TestCancelledApplyDoesNotWrite(t *testing.T) {
	calls := 0
	s := fixture(t, Options{WriteEnabled: true, Validate: func(context.Context, string) error { return nil }, Apply: func(context.Context) error { calls++; return nil }})
	snap, _ := s.Get()
	p, _ := s.Preview(context.Background(), "admin", snap.Revision, snap.DNS)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Apply(ctx, "admin", p.Token); err == nil {
		t.Fatal("cancel ignored")
	}
	b, _ := os.ReadFile(s.opts.Path)
	if string(b) != sample || calls != 0 {
		t.Fatal("cancel had side effects")
	}
}
func TestRollbackPreservesConcurrentExternalChange(t *testing.T) {
	var configPath string
	s := fixture(t, Options{WriteEnabled: true, Validate: func(context.Context, string) error { return nil }, Apply: func(context.Context) error {
		_ = os.WriteFile(configPath, []byte(sample+"# external update\n"), 0600)
		return errors.New("fail")
	}})
	configPath = s.opts.Path
	snap, _ := s.Get()
	p, _ := s.Preview(context.Background(), "admin", snap.Revision, snap.DNS)
	if _, err := s.Apply(context.Background(), "admin", p.Token); err == nil || err.Error() != "configuration_rollback_conflict" {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.opts.Path)
	if !strings.Contains(string(b), "external update") {
		t.Fatal("external edit overwritten")
	}
}
func TestFailedHealthRollsBack(t *testing.T) {
	applies, checks := 0, 0
	s := fixture(t, Options{WriteEnabled: true, Validate: func(context.Context, string) error { return nil }, Apply: func(context.Context) error { applies++; return nil }, Verify: func(context.Context) error {
		checks++
		if checks == 1 {
			return errors.New("unhealthy")
		}
		return nil
	}})
	snap, _ := s.Get()
	p, _ := s.Preview(context.Background(), "admin", snap.Revision, snap.DNS)
	if _, err := s.Apply(context.Background(), "admin", p.Token); !errors.Is(err, ErrApply) {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.opts.Path)
	if string(b) != sample || applies != 2 || checks != 2 {
		t.Fatal("health rollback not completed")
	}
}
func TestInvalidDNSAndSymlink(t *testing.T) {
	if validateDNS(DNS{Domain: "bad domain"}) == nil || validateDNS(DNS{Domain: "mesh.internal", Servers: []string{"999.1.1.1"}}) == nil {
		t.Fatal("invalid DNS accepted")
	}
	if _, e := New(Options{WriteEnabled: true, Path: filepath.Join(t.TempDir(), "c.yaml")}); e == nil {
		t.Fatal("unsafe write config allowed")
	}
}
