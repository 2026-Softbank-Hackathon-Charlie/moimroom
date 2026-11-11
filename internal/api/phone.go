package api

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
)

// phoneHashIter makes guessing a number from its hash costly: Korean mobile
// numbers have only about 10^8 candidates, so a plain hash would not protect them.
const phoneHashIter = 100_000

// normalizePhone keeps digits and accepts Korean mobile numbers (01X, 10 or
// 11 digits).
func normalizePhone(raw string) (string, bool) {
	var b strings.Builder
	for _, c := range raw {
		switch {
		case c >= '0' && c <= '9':
			b.WriteRune(c)
		case c == '-' || c == ' ':
		default:
			return "", false
		}
	}
	d := b.String()
	return d, (len(d) == 10 || len(d) == 11) && strings.HasPrefix(d, "01")
}

// matchesPrivateHash checks the existing salted PBKDF2 representation. Neither
// the phone nor the canonical request is persisted as a fast dictionary hash.
func matchesPrivateHash(value, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || parts[1] != "100000" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) != 16 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) != 32 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, value, salt, phoneHashIter, 32)
	return err == nil && subtle.ConstantTimeCompare(want, got) == 1
}

// hashPhone returns a salted PBKDF2-SHA256 hash in a self-describing format.
func hashPhone(digits string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, digits, salt, phoneHashIter, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", phoneHashIter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}
