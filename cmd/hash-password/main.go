// hash-password reads a password from a terminal or private stdin, never argv.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"golang.org/x/term"
	"io"
	"meshconsole/internal/auth"
	"os"
	"strings"
	"unicode/utf8"
)

func readPassword(in *os.File, out io.Writer) (string, error) {
	if term.IsTerminal(int(in.Fd())) {
		fmt.Fprint(out, "Password: ")
		first, e := term.ReadPassword(int(in.Fd()))
		fmt.Fprintln(out)
		if e != nil {
			return "", errors.New("cannot read password")
		}
		fmt.Fprint(out, "Confirm: ")
		second, e := term.ReadPassword(int(in.Fd()))
		fmt.Fprintln(out)
		if e != nil {
			return "", errors.New("cannot read confirmation")
		}
		if string(first) != string(second) {
			return "", errors.New("password confirmation mismatch")
		}
		return string(first), nil
	}
	b, e := io.ReadAll(io.LimitReader(bufio.NewReader(in), 4097))
	if e != nil || len(b) > 4096 {
		return "", errors.New("invalid stdin password")
	}
	s := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	if strings.ContainsAny(s, "\r\n") {
		return "", errors.New("stdin must contain one password line")
	}
	return s, nil
}
func run() error {
	if len(os.Args) != 1 {
		return errors.New("password arguments are forbidden; use interactive input or secure stdin")
	}
	pw, e := readPassword(os.Stdin, os.Stderr)
	if e != nil {
		return e
	}
	if !utf8.ValidString(pw) || utf8.RuneCountInString(pw) < 12 || len(pw) > 1024 {
		return errors.New("password must be at least 12 characters and at most 1024 bytes")
	}
	hash, e := auth.HashPassword(pw)
	if e != nil {
		return e
	}
	fmt.Fprintln(os.Stdout, hash)
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(1)
	}
}
