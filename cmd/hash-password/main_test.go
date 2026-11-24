package main

import (
	"bytes"
	"meshconsole/internal/auth"
	"os"
	"path/filepath"
	"testing"
)

func TestSecureStdinAndValidation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "stdin")
	if e := os.WriteFile(p, []byte("secure-password-123\n"), 0600); e != nil {
		t.Fatal(e)
	}
	f, e := os.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	var out bytes.Buffer
	pw, e := readPassword(f, &out)
	if e != nil || pw != "secure-password-123" || out.Len() != 0 {
		t.Fatal("stdin leaks or invalid")
	}
	hash, e := auth.HashPassword(pw)
	if e != nil {
		t.Fatal(e)
	}
	ok, e := auth.VerifyPassword(pw, hash)
	if e != nil || !ok {
		t.Fatal("hash")
	}
}
func TestRejectArgumentsAndMultipleLines(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"hash-password", "plaintext"}
	if e := run(); e == nil {
		t.Fatal("argv accepted")
	}
	p := filepath.Join(t.TempDir(), "stdin")
	os.WriteFile(p, []byte("first-password\nsecond-password\n"), 0600)
	f, e := os.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, e = readPassword(f, &bytes.Buffer{}); e == nil {
		t.Fatal("multiple stdin lines accepted")
	}
}
