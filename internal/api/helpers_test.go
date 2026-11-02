package api

import (
	"strings"
	"testing"
)

func TestNormalizePhone(t *testing.T) {
	for in, want := range map[string]string{
		"010-1234-5678": "01012345678",
		"01012345678":   "01012345678",
		"011 234 5678":  "0112345678",
	} {
		got, ok := normalizePhone(in)
		if !ok || got != want {
			t.Errorf("%q: got %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "02-123-4567", "010-1234-567a", "0101234567890", "1234"} {
		if _, ok := normalizePhone(in); ok {
			t.Errorf("%q should be rejected", in)
		}
	}
}

func TestHashPhone(t *testing.T) {
	a, err := hashPhone("01012345678")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := hashPhone("01012345678")
	if a == b {
		t.Fatal("hashes must be salted")
	}
	if !strings.HasPrefix(a, "pbkdf2-sha256$100000$") || strings.Contains(a, "01012345678") {
		t.Fatalf("unexpected hash format %q", a)
	}
}

func TestMaskName(t *testing.T) {
	for in, want := range map[string]string{"김민수": "김*수", "이준": "이*", "남궁민수": "남**수", "A": "*", "": ""} {
		if got := maskName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	for in, want := range map[string]string{"k7qm29xa": "K7QM-29XA", " K7QM-29XA ": "K7QM-29XA", "k7qm 29xa": "K7QM-29XA", "abc": "ABC"} {
		if got := normalizeCode(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
