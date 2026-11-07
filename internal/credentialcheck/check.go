// Package credentialcheck reports fresh, read-only dependency operations made
// by the application with its own runtime credentials. It never returns values,
// endpoints, account names or provider errors.
package credentialcheck

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"
)

var ErrNotConfigured = errors.New("credential check not configured")
var nonce = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Operation func(context.Context, string) error

// Handler runs at most one verification at a time. Operations must be bounded,
// read-only, and use the same configuration as normal application requests.
func Handler(database, bucket Operation) http.Handler {
	busy := make(chan struct{}, 1)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		challenge := r.URL.Query().Get("challenge")
		if len(r.URL.Query()["challenge"]) != 1 || !nonce.MatchString(challenge) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case busy <- struct{}{}:
			defer func() { <-busy }()
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		checks := map[string]string{}
		for name, op := range map[string]Operation{"database-query": database, "bucket-access": bucket} {
			status := "unknown"
			if op != nil && ctx.Err() == nil {
				err := op(ctx, challenge)
				switch {
				case ctx.Err() != nil || errors.Is(err, ErrNotConfigured):
				case err != nil:
					status = "fail"
				default:
					status = "pass"
				}
			}
			checks[name] = status
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Version   int               `json:"version"`
			Challenge string            `json:"challenge"`
			Checks    map[string]string `json:"checks"`
		}{1, challenge, checks})
	})
}
