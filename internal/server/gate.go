package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
)

// Two passwords, one gate.
//
// This app has no accounts, so a password is the whole difference between a capability and
// an open door. There are exactly two, and they are separate because they answer different
// questions:
//
//	BA_PASS     may this change what the engine will say?
//	ADMIN_PASS  may this see how the instance is configured?
//
// Neither is an identity. Reads stay open, because "anyone on the tailnet can read the
// documents" is the point of the product; the gates exist so "anyone on the tailnet can
// rewrite them" and "…can read the shape of every secret on the box" are not.
//
// An unset password means the surface it guards does not exist — 403, and for the admin
// screen the route is never registered at all. Forgetting to configure a secret must never
// be how you end up without one.
//
// One implementation on purpose: a second constant-time compare is a second place to get
// subtly wrong, and the 403-versus-401 distinction is a decision, not a detail. 403 says
// there is nothing to unlock on this instance; 401 says try a different password.
type gate struct {
	value  string // the configured secret; empty disables the surface
	header string // the request header that carries it
	env    string // the variable an operator sets, named in the errors
	what   string // the surface, for the 403: "writes", "the admin screen"
}

// The two request headers, named once. The BA one appears in every write test as well, and
// goconst is right to want a single home for a string the client and the server must spell
// identically — a typo in one of eight copies is a 401 nobody can explain.
const (
	headerBA    = "X-BA-Pass"
	headerAdmin = "X-Admin-Pass"
)

func (g gate) enabled() bool { return g.value != "" }

func (g gate) wrap(h http.HandlerFunc) http.HandlerFunc {
	if !g.enabled() {
		msg := fmt.Sprintf("%s: %s is not set on this instance", g.what, g.env)
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, msg, http.StatusForbidden)
		}
	}
	want := sha256.Sum256([]byte(g.value))
	return func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get(g.header)))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			http.Error(w, "wrong "+g.env, http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// BAPass gates every action that changes what the engine will say: confirming an answer
// into the corpus, dismissing a question, importing a document and removing one.
type BAPass string

func (p BAPass) gate() gate {
	return gate{value: string(p), header: headerBA, env: "BA_PASS", what: "writes are disabled"}
}

func (p BAPass) enabled() bool { return p.gate().enabled() }

// AdminPass gates the Admin screen's one endpoint. Separate from BA_PASS because the
// question is different: the settings list carries the *provenance* of every password on the
// box, so a BA who may publish an answer is not thereby someone who may read that.
type AdminPass string

func (p AdminPass) gate() gate {
	return gate{
		value: string(p), header: headerAdmin, env: "ADMIN_PASS",
		what: "the admin screen is disabled",
	}
}

func (p AdminPass) enabled() bool { return p.gate().enabled() }
