package web

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

/* ══ The front end's critical rules, as tests ══════════════════════════════════
   The architecture is four layers (see CLAUDE.md). Three of its rules can be broken
   without anything failing at build time and without a browser looking wrong until the
   exact moment someone uses the feature. Those three are checked here, in Go, because
   this repo already tests its own HTML from Go and because a rule that only a human
   remembers is not a rule.

   What is deliberately *not* here: a JavaScript linter. Measured on this tree,
   @antfu/eslint-config pulls 255 packages and 113 MB to report three findings, all of
   them small (two unused capturing groups and a `parseFloat`), and its best rules —
   eslint-plugin-vue's reactivity-loss checks — do not fire at all without SFCs. The day
   TypeScript or .vue files land here, that config is the right call and this comment is
   the trigger. Until then these three tests carry the load for free.
   ═══════════════════════════════════════════════════════════════════════════ */

// The layers, by file. Everything under web/app is in exactly one of them.
var (
	// Plumbing: fetch, SSE, storage, markdown, DOM maths. No Vue, so each of these runs
	// in a bare console and is testable without mounting anything.
	plumbing = []string{
		"chat.js", "qa.js", "upload.js", "answer.js", "diagram.js",
		"library.js", "session.js", "viewport.js",
	}
	// Components: props, emits, compose, return. No branches.
	components = []string{"ba.js", "tree.js"}
)

// Rule 1 — the plumbing layer may not touch Vue.
//
// It is what makes those files testable in isolation and reusable from anywhere: the
// moment one of them reaches for a `ref`, it can only run inside a mounted app, and the
// seam that keeps `app.js` free of AbortControllers and TextDecoders is gone.
func TestPlumbingDoesNotImportVue(t *testing.T) {
	for _, name := range plumbing {
		src := readApp(t, name)
		for _, bad := range []string{"Vue.", "from \"vue\"", "= Vue"} {
			if strings.Contains(src, bad) {
				t.Errorf("web/app/%s reaches for Vue (%q). Plumbing must stay framework-free — "+
					"put the reactive part in a composable under web/app/use/ instead.", name, bad)
			}
		}
	}
}

// Rule 2 — a composable may not reach for another composable's state.
//
// What it needs arrives as an argument, so each file can be read alone and the shell
// stays the only place that knows the whole picture. An import between two of them is how
// that becomes a graph nobody can hold in their head.
func TestComposablesDoNotImportEachOther(t *testing.T) {
	imports := regexp.MustCompile(`(?m)^import .*?from "([^"]+)"`)
	for _, path := range appFiles(t, "use") {
		src := readFile(t, path)
		for _, m := range imports.FindAllStringSubmatch(src, -1) {
			if strings.Contains(m[1], "use/") || (!strings.HasPrefix(m[1], "../") && strings.HasPrefix(m[1], "./")) {
				t.Errorf("%s imports %q — a composable takes what it needs as an argument, "+
					"never another composable's state.", filepath.Base(path), m[1])
			}
		}
	}
}

// Rule 3 — a component holds no branches.
//
// A component declares what it needs (props, emits), composes the behaviour, and returns
// it. The first `if` is the signal that a composable is missing: that is exactly how
// ba.js grew to 179 lines holding the password gate, the import pipeline and the ticket
// transitions at once.
func TestComponentsHoldNoLogic(t *testing.T) {
	// Ternaries and optional chaining are not branches in this sense — they read as one
	// expression. A statement-level branch is.
	branches := regexp.MustCompile(`\b(if|for|while|switch)\s*\(`)
	for _, name := range components {
		src := readApp(t, name)
		if found := branches.FindAllString(stripComments(src), -1); len(found) > 0 {
			t.Errorf("web/app/%s contains %v — a component with a branch is a composable "+
				"nobody wrote yet. Move it to web/app/use/.", name, found)
		}
	}
}

// Rule 4 — everything the templates bind must exist somewhere in the code behind them.
//
// This is the one the new architecture made possible: `setup()` returns an object, and a
// key that is missing from it is `undefined` at render with no error, no warning and no
// failed build — just a blank where a badge should be. The check is deliberately coarse
// (does the identifier appear at all in the module graph behind that template?) because a
// coarse check that runs is worth more than an exact one that needs a JS parser.
func TestTemplatesBindNothingUndefined(t *testing.T) {
	html := readFile(t, "index.html")
	appHTML, baHTML := splitTemplates(t, html)

	for _, c := range []struct {
		name  string
		html  string
		files []string
	}{
		// The shell's template is backed by app.js and everything it wires.
		{"#app", appHTML, append([]string{"app.js"}, base(appFiles(t, "use"))...)},
		// The BA screen's template is backed by its component and the three behind it.
		{"#ba-screen", baHTML, []string{"ba.js", "use/gate.js", "use/importer.js", "use/tickets.js", "qa.js"}},
	} {
		var behind strings.Builder
		for _, f := range c.files {
			behind.WriteString(readApp(t, f))
		}
		code := behind.String()

		for _, id := range bound(c.html) {
			if !strings.Contains(code, id) {
				t.Errorf("%s binds %q and nothing behind it defines that name — a missing key in a "+
					"setup() return is undefined at render, with no error anywhere. Files checked: %v",
					c.name, id, c.files)
			}
		}
	}
}

/* ── helpers ─────────────────────────────────────────────────────────────────── */

// splitTemplates cuts index.html at the BA screen's <template>, so an identifier is
// checked against the code that actually backs the markup binding it.
func splitTemplates(t *testing.T, html string) (appHTML, baHTML string) {
	t.Helper()
	const marker = `<template id="ba-screen">`
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatalf("index.html no longer contains %s — this test's assumption about where the "+
			"BA markup lives is stale, not the code", marker)
	}
	start := strings.Index(html, `<div id="app"`)
	if start < 0 {
		t.Fatal(`index.html no longer contains <div id="app"`)
	}
	return html[start:i], html[i:]
}

var (
	// Vue bindings: a mustache, or an attribute that carries an expression.
	mustache = regexp.MustCompile(`\{\{([^}]+)\}\}`)
	bindAttr = regexp.MustCompile(`(?:@[\w:.-]+|:[\w-]+|v-(?:if|else-if|for|show|html|model)(?::[\w-]+)?)="([^"]*)"`)
	word     = regexp.MustCompile(`[A-Za-z_$][\w$]*`)
	// v-for="x in xs" and v-for="(x, i) in xs" declare their own names.
	vFor = regexp.MustCompile(`v-for="\(?([^)]*?)\)? in `)
	// String literals in an expression. Single quotes inside an attribute value, and
	// double quotes inside a mustache — where they are legal, because a mustache sits in
	// text rather than in an attribute.
	quoted = regexp.MustCompile(`'[^']*'|"[^"]*"`)
)

// bound lists the identifiers a template expects the code behind it to provide: the head
// of every expression, minus the names the template declares itself and the ones the
// language provides.
func bound(html string) []string {
	locals := map[string]bool{}
	for _, m := range vFor.FindAllStringSubmatch(html, -1) {
		for _, name := range word.FindAllString(m[1], -1) {
			locals[name] = true
		}
	}

	seen := map[string]bool{}
	var out []string
	add := func(expr string) {
		// A string literal is not a binding. `:class="t.ok ? 'tip' : 'memo'"` names two
		// CSS classes, not two things setup() must return.
		expr = quoted.ReplaceAllString(expr, " ")
		// Position matters: only the *head* of a member expression is the app's to define.
		// In `statusLine.tokens` and `$event.dataTransfer.files`, everything after a dot
		// belongs to the object, not to setup().
		for _, at := range word.FindAllStringIndex(expr, -1) {
			if at[0] > 0 && expr[at[0]-1] == '.' {
				continue
			}
			name := expr[at[0]:at[1]]
			if locals[name] || jsGlobals[name] || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, m := range mustache.FindAllStringSubmatch(html, -1) {
		add(m[1])
	}
	for _, m := range bindAttr.FindAllStringSubmatch(html, -1) {
		add(m[1])
	}
	sort.Strings(out)
	return out
}

// jsGlobals is what the language and the DOM provide, plus the handler argument Vue
// passes. Nothing here has to come from setup().
var jsGlobals = map[string]bool{
	"true": true, "false": true, "null": true, "undefined": true, "NaN": true,
	"String": true, "Number": true, "Boolean": true, "Object": true, "Array": true,
	"JSON": true, "Math": true, "Date": true, "event": true,
	"length": true, "trim": true, "toLocaleString": true, "target": true, "detail": true,
	"value": true, "open": true, "files": true, "key": true, "in": true, "of": true,
	// Vue's own instance properties, available to every template without being returned.
	"$emit": true, "$event": true, "$refs": true, "emit": true,
}

func readApp(t *testing.T, name string) string {
	t.Helper()
	return readFile(t, filepath.Join("app", name))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// appFiles lists the .js files in web/app/<dir>, so a new composable is covered by these
// rules the moment it exists rather than when someone remembers to add it here.
func appFiles(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("app", dir, "*.js"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no .js files under app/%s (%v)", dir, err)
	}
	sort.Strings(paths)
	return paths
}

func base(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, filepath.Join("use", filepath.Base(p)))
	}
	return out
}

// stripComments removes // and /* */ so a rule is not tripped by the word "if" in a
// sentence explaining why there is no if.
func stripComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	out := b.String()
	for {
		i := strings.Index(out, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(out[i:], "*/")
		if j < 0 {
			break
		}
		out = out[:i] + out[i+j+2:]
	}
	return out
}
