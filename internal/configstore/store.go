// Package configstore manages only DNS fields in one operator-configured YAML file.
package configstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type DNS struct {
	Magic    bool     `json:"magic"`
	Domain   string   `json:"domain"`
	Servers  []string `json:"servers"`
	Override bool     `json:"override"`
}
type Snapshot struct {
	DNS             DNS    `json:"dns"`
	Revision        string `json:"revision"`
	Writable        bool   `json:"writable"`
	Configured      bool   `json:"configured"`
	RequiresRestart bool   `json:"requiresRestart"`
}
type Plan struct {
	Token           string    `json:"token"`
	Revision        string    `json:"revision"`
	Before          DNS       `json:"before"`
	After           DNS       `json:"after"`
	ExpiresAt       time.Time `json:"expiresAt"`
	RequiresRestart bool      `json:"requiresRestart"`
}
type Options struct {
	Path                 string
	WriteEnabled         bool
	ConfigTestExecutable string
	ApplyExecutable      string
	// Hooks are only programmatic injection for tests; no API accepts commands.
	Validate func(context.Context, string) error
	Apply    func(context.Context) error
	Verify   func(context.Context) error
}
type pending struct {
	plan  Plan
	bytes []byte
	owner string
}
type Store struct {
	mu    sync.Mutex
	opts  Options
	plans map[string]pending
}

var ErrConflict = errors.New("configuration_conflict")
var ErrReadOnly = errors.New("configuration_read_only")
var ErrUnavailable = errors.New("configuration_unavailable")
var ErrApply = errors.New("configuration_apply_failed")
var domainPattern = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z][a-zA-Z0-9-]{0,62}$`)

func New(opts Options) (*Store, error) {
	if opts.Path != "" && !filepath.IsAbs(opts.Path) {
		return nil, errors.New("configuration path must be absolute")
	}
	if opts.WriteEnabled {
		if opts.Path == "" || (opts.Validate == nil && opts.ConfigTestExecutable == "") || (opts.Apply == nil && opts.ApplyExecutable == "") {
			return nil, errors.New("configuration writes require validation and apply executables")
		}
		for _, p := range []string{opts.ConfigTestExecutable, opts.ApplyExecutable} {
			if p != "" && !filepath.IsAbs(p) {
				return nil, errors.New("trusted executable must have an absolute path")
			}
		}
	}
	return &Store{opts: opts, plans: make(map[string]pending)}, nil
}
func revision(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func mapping(n *yaml.Node, key string) *yaml.Node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
func set(n *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			n.Content[i+1] = v
			return
		}
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
}
func scalar(v any) *yaml.Node { n := &yaml.Node{}; _ = n.Encode(v); return n }
func readDNS(root *yaml.Node) (DNS, error) {
	d := mapping(root, "dns")
	if d == nil || d.Kind != yaml.MappingNode {
		return DNS{}, errors.New("dns configuration missing")
	}
	var raw struct {
		Magic    bool      `yaml:"magic_dns"`
		Domain   string    `yaml:"base_domain"`
		Override bool      `yaml:"override_local_dns"`
		Servers  yaml.Node `yaml:"nameservers"`
	}
	if err := d.Decode(&raw); err != nil {
		return DNS{}, ErrUnavailable
	}
	out := DNS{Magic: raw.Magic, Domain: raw.Domain, Override: raw.Override, Servers: []string{}}
	ns := mapping(d, "nameservers")
	if ns != nil {
		if ns.Kind == yaml.SequenceNode {
			_ = ns.Decode(&out.Servers)
		} else if ns.Kind == yaml.MappingNode {
			if g := mapping(ns, "global"); g != nil {
				_ = g.Decode(&out.Servers)
			}
		}
	}
	return out, nil
}
func (s *Store) read() ([]byte, *yaml.Node, error) {
	if s.opts.Path == "" {
		return nil, nil, ErrUnavailable
	}
	info, err := os.Lstat(s.opts.Path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, nil, ErrUnavailable
	}
	b, err := os.ReadFile(s.opts.Path)
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	var doc yaml.Node
	if yaml.Unmarshal(b, &doc) != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, ErrUnavailable
	}
	// YAML aliases can cause unintended shared-field mutation; reject for writes.
	return b, &doc, nil
}
func (s *Store) Get() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, doc, err := s.read()
	if err != nil {
		return Snapshot{Configured: false, RequiresRestart: true}, err
	}
	dns, err := readDNS(doc.Content[0])
	return Snapshot{DNS: dns, Revision: revision(b), Writable: s.opts.WriteEnabled, Configured: true, RequiresRestart: true}, err
}
func validateDNS(d DNS) error {
	if len(d.Domain) > 253 || !domainPattern.MatchString(d.Domain) {
		return errors.New("invalid_dns_domain")
	}
	if len(d.Servers) > 8 {
		return errors.New("too_many_dns_servers")
	}
	for _, v := range d.Servers {
		if _, e := netip.ParseAddr(v); e != nil {
			return errors.New("invalid_dns_server")
		}
	}
	return nil
}
func noAliases(n *yaml.Node) bool {
	if n.Kind == yaml.AliasNode {
		return false
	}
	for _, v := range n.Content {
		if !noAliases(v) {
			return false
		}
	}
	return true
}
func (s *Store) Preview(ctx context.Context, owner, rev string, d DNS) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Plan{}, errors.New("configuration_request_cancelled")
	}
	if !s.opts.WriteEnabled {
		return Plan{}, ErrReadOnly
	}
	if err := validateDNS(d); err != nil {
		return Plan{}, err
	}
	b, doc, err := s.read()
	if err != nil {
		return Plan{}, err
	}
	if revision(b) != rev {
		return Plan{}, ErrConflict
	}
	if !noAliases(doc) {
		return Plan{}, errors.New("yaml_aliases_not_supported")
	}
	info, statErr := os.Lstat(s.opts.Path)
	if statErr != nil {
		return Plan{}, ErrUnavailable
	}
	if ownerErr := checkOwner(info); ownerErr != nil {
		return Plan{}, ownerErr
	}
	before, err := readDNS(doc.Content[0])
	if err != nil {
		return Plan{}, err
	}
	dns := mapping(doc.Content[0], "dns")
	set(dns, "magic_dns", scalar(d.Magic))
	set(dns, "base_domain", scalar(d.Domain))
	set(dns, "override_local_dns", scalar(d.Override))
	ns := mapping(dns, "nameservers")
	if ns != nil && ns.Kind == yaml.MappingNode {
		set(ns, "global", scalar(d.Servers))
	} else {
		// Supported Headscale versions use nameservers.global. Migrate legacy [] safely.
		global := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		set(global, "global", scalar(d.Servers))
		set(dns, "nameservers", global)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if enc.Encode(doc) != nil {
		return Plan{}, ErrUnavailable
	}
	_ = enc.Close()
	candidate, err := os.CreateTemp(filepath.Dir(s.opts.Path), ".mesh-validate-*.yaml")
	if err != nil {
		return Plan{}, ErrUnavailable
	}
	tmp := candidate.Name()
	defer os.Remove(tmp)
	_ = candidate.Chmod(0600)
	if _, err = candidate.Write(buf.Bytes()); err != nil {
		candidate.Close()
		return Plan{}, ErrUnavailable
	}
	if candidate.Close() != nil {
		return Plan{}, ErrUnavailable
	}
	if err = s.runValidate(ctx, tmp); err != nil {
		return Plan{}, errors.New("configuration_validation_failed")
	}
	now := time.Now()
	for k, p := range s.plans {
		if now.After(p.plan.ExpiresAt) {
			delete(s.plans, k)
		}
	}
	if len(s.plans) >= 32 {
		return Plan{}, errors.New("configuration_plan_capacity")
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return Plan{}, ErrUnavailable
	}
	plan := Plan{Token: hex.EncodeToString(token), Revision: rev, Before: before, After: d, ExpiresAt: now.Add(2 * time.Minute), RequiresRestart: true}
	s.plans[plan.Token] = pending{plan: plan, bytes: append([]byte(nil), buf.Bytes()...), owner: owner}
	return plan, nil
}
func (s *Store) runValidate(ctx context.Context, candidate string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if s.opts.Validate != nil {
		return s.opts.Validate(ctx, candidate)
	}
	cmd := exec.CommandContext(ctx, s.opts.ConfigTestExecutable, "--config", candidate, "configtest")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HOME=" + os.Getenv("HOME")}
	return cmd.Run()
}
func (s *Store) runApply(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if s.opts.Apply != nil {
		return s.opts.Apply(ctx)
	}
	cmd := exec.CommandContext(ctx, s.opts.ApplyExecutable)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HOME=" + os.Getenv("HOME")}
	return cmd.Run()
}
func atomicWrite(p string, b []byte, mode os.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(p), ".mesh-write-*.yaml")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode.Perm()); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, p)
}
func (s *Store) Apply(ctx context.Context, owner, token string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, errors.New("configuration_request_cancelled")
	}
	if !s.opts.WriteEnabled {
		return Snapshot{}, ErrReadOnly
	}
	p, ok := s.plans[token]
	delete(s.plans, token)
	if !ok || p.owner != owner || time.Now().After(p.plan.ExpiresAt) {
		return Snapshot{}, errors.New("invalid_configuration_plan")
	}
	old, _, err := s.read()
	if err != nil {
		return Snapshot{}, err
	}
	if revision(old) != p.plan.Revision {
		return Snapshot{}, ErrConflict
	}
	info, err := os.Lstat(s.opts.Path)
	if err != nil || !info.Mode().IsRegular() {
		return Snapshot{}, ErrUnavailable
	}
	if ownerErr := checkOwner(info); ownerErr != nil {
		return Snapshot{}, ownerErr
	}
	if ctx.Err() != nil {
		return Snapshot{}, errors.New("configuration_request_cancelled")
	}
	backup := fmt.Sprintf("%s.mesh-backup-%d", s.opts.Path, time.Now().UnixNano())
	if err = os.WriteFile(backup, old, 0600); err != nil {
		return Snapshot{}, ErrUnavailable
	}
	if err = atomicWrite(s.opts.Path, p.bytes, info.Mode()); err != nil {
		return Snapshot{}, ErrUnavailable
	}
	err = s.runApply(ctx)
	if err == nil && s.opts.Verify != nil {
		vctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = s.opts.Verify(vctx)
		cancel()
	}
	if err != nil {
		current, _, readErr := s.read()
		if readErr != nil || revision(current) != revision(p.bytes) {
			return Snapshot{}, errors.New("configuration_rollback_conflict")
		}
		if atomicWrite(s.opts.Path, old, info.Mode()) != nil {
			return Snapshot{}, errors.New("configuration_rollback_failed")
		}
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		if s.runApply(recoveryCtx) != nil {
			return Snapshot{}, errors.New("configuration_rollback_apply_failed")
		}
		if s.opts.Verify != nil && s.opts.Verify(recoveryCtx) != nil {
			return Snapshot{}, errors.New("configuration_rollback_health_failed")
		}
		return Snapshot{}, ErrApply
	}
	return Snapshot{DNS: p.plan.After, Revision: revision(p.bytes), Writable: true, Configured: true, RequiresRestart: true}, nil
}
