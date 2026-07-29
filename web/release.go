package web

import (
	_ "embed" // release.json is embedded below; the package itself needs no embed symbol
	"encoding/json"
	"fmt"
)

// This file is the release the binary was cut from: a semantic version and what changed
// since the one before it, generated into release.json by `make release` and embedded here.
//
// It answers a different question from the commit in /api/health, which is why both exist
// and neither replaces the other. The commit says *which bytes are running* — exact, always
// available, meaningless to a reader. The release says *what changed*, which nobody can get
// to from a sha. So the badge shows the release and the modal behind it shows the notes,
// while the commit stays the identity underneath.
//
// The number is never edited by hand. `make release V=v0.13.0` creates an annotated tag and
// generates this file from `git log` in the same commit the tag points at. A VERSION file
// was rejected for the reason recorded in changelog/2026-07-28-deploy-and-version.md — a
// version somebody has to remember to bump is a version that lies — and a tag defeats that
// objection rather than repeating it: forget to cut one and Version is simply empty, which
// makes the UI fall back to the commit instead of asserting a stale number.
//
//go:embed release.json
var releaseJSON []byte

// ReleaseNote is one commit, parsed from its Conventional-Commit subject. A subject that
// follows no convention keeps Kind "other" and its full Subject rather than being dropped:
// a release that silently omits commits is worse than one with an untidy line in it.
type ReleaseNote struct {
	Kind    string `json:"kind"`
	Scope   string `json:"scope"`
	Subject string `json:"subject"`
	Commit  string `json:"commit"`
}

// Release is what release.json records. Version is empty in a tree with no tags, and every
// reader treats that as "fall back to the commit" rather than as an error.
type Release struct {
	Version  string        `json:"version"`
	Date     string        `json:"date"`
	Previous string        `json:"previous"`
	Notes    []ReleaseNote `json:"notes"`
}

// ReleaseInfo parses that stamp. Only the version reaches /api/health — the notes are a
// separate route, fetched when somebody opens the modal, because a payload that grows with
// every commit has no business in the endpoint every client polls on reconnect.
func ReleaseInfo() (Release, error) {
	var r Release
	if err := json.Unmarshal(releaseJSON, &r); err != nil {
		return r, fmt.Errorf("web: web/release.json is not valid JSON: %w", err)
	}
	return r, nil
}

// ReleaseJSON is the same bytes, served verbatim by GET /api/release. Serving the embedded
// file rather than re-marshalling ReleaseInfo keeps one shape on the wire: a field added to
// the generator reaches the browser without a Go struct learning about it first.
func ReleaseJSON() []byte { return releaseJSON }
