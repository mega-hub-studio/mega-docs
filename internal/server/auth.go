package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// Auth is HTTP Basic credentials. The zero value means no authentication, which
// is only safe on loopback or inside a private network (a tailnet, a VPN) where
// something else already decides who reaches the port.
//
// Basic auth on purpose: the browser owns the prompt, phones remember it in the
// keychain, and there is no login page, no session store and no CSRF surface to
// get wrong. Over HTTPS it is enough for an internal read-only tool. Anything
// stronger belongs at the edge — Cloudflare Access or a tailnet ACL — not here.
type Auth struct {
	User string
	Pass string
}

func (a Auth) enabled() bool { return a.Pass != "" }

// Enabled reports whether credentials are configured, so callers can warn about
// an unprotected exposure without handling the password itself.
func (a Auth) Enabled() bool { return a.enabled() }

// guard requires credentials on everything except /api/health, which stays open
// so tunnels, load balancers and uptime checks can probe without a secret. It
// reveals nothing but liveness.
func guard(a Auth, h http.Handler) http.Handler {
	if !a.enabled() {
		return h
	}
	// Compare digests, not the raw strings: equal-length hashes keep
	// ConstantTimeCompare from leaking the credential's length.
	wantUser := sha256.Sum256([]byte(a.User))
	wantPass := sha256.Sum256([]byte(a.Pass))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			h.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if ok {
			gotUser := sha256.Sum256([]byte(user))
			gotPass := sha256.Sum256([]byte(pass))
			if subtle.ConstantTimeCompare(gotUser[:], wantUser[:]) == 1 &&
				subtle.ConstantTimeCompare(gotPass[:], wantPass[:]) == 1 {
				h.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="Knowledge Engine", charset="UTF-8"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}
