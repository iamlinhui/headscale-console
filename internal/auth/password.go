package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"golang.org/x/crypto/scrypt"
	"meshconsole/internal/config"
	"strings"
)

func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", errors.New("password salt generation failed")
	}
	key, e := scrypt.Key([]byte(pw), salt, 16384, 8, 1, 64)
	if e != nil {
		return "", errors.New("password hashing failed")
	}
	return "scrypt$16384$8$1$" + base64.StdEncoding.EncodeToString(salt) + "$" + base64.StdEncoding.EncodeToString(key), nil
}
func VerifyPassword(pw, encoded string) (bool, error) {
	if e := config.ValidatePasswordHash(encoded); e != nil {
		return false, e
	}
	p := strings.Split(encoded, "$")
	salt, _ := base64.StdEncoding.DecodeString(p[4])
	expected, _ := base64.StdEncoding.DecodeString(p[5])
	actual, e := scrypt.Key([]byte(pw), salt, 16384, 8, 1, 64)
	if e != nil {
		return false, errors.New("password verification failed")
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
