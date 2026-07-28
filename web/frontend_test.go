package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

/* ══ The front end's layer rules, as tests ══════════════════════════════════════
   The app is built by Vite from web/ui (Vue 3.5 SFCs, JavaScript — no TypeScript, by
   choice). Its architecture is four layers:

     lib/          plumbing: fetch, SSE, storage, markdown, DOM maths. No Vue.
     composables/  every branch and every piece of reactive state, one concern each
     components/   *.vue — props, emits, compose, template. No branches in the script.
     App.vue       wiring: who gets what

   Four rules can be broken without the build failing and without a browser looking wrong
   until the moment somebody uses the feature, so they are checked here.

   A fifth used to be — "everything a template binds exists behind it" — and it moved to
   ESLint when the SFCs landed: `vue/no-undef-properties` reads a real Vue parse of a real
   component, which is strictly better than the regex over module text that approximated
   it. That is `make lint-js`, and it runs inside `make check` now that there are .vue
   files for it to be good at. The trigger written in the old comment was exactly this.
   ═══════════════════════════════════════════════════════════════════════════ */

const uiSrc = "ui/src" // relative to web/, which is this package's directory

// Rule — the plumbing layer may not touch Vue.
//
// It is what makes those files testable in isolation and reusable from anywhere: the
// moment one of them reaches for a `ref`, it can only run inside a mounted app, and the
// seam that keeps App.vue free of AbortControllers and TextDecoders is gone.
func TestPlumbingDoesNotImportVue(t *testing.T) {
	files := filesIn(t, filepath.Join(uiSrc, "lib"), ".js")
	if len(files) < 6 {
		t.Fatalf("only %d files under %s/lib — has the layer moved?", len(files), uiSrc)
	}
	// This rule asserts an *absence*, so a pattern that matches nothing at all passes it
	// for the wrong reason. The composables layer is the positive control: every file in it
	// imports from vue by definition, so if the pattern finds none there it is broken, not
	// satisfied. (It was, once — a reformat to single quotes silently disarmed it.)
	control := filesIn(t, filepath.Join(uiSrc, "composables"), ".js")
	matched := 0
	for _, path := range control {
		if reImportsVue.MatchString(readFile(t, path)) {
			matched++
		}
	}
	if matched == 0 {
		t.Fatalf("reImportsVue matched none of the %d composables, which all import from vue — "+
			"the pattern no longer matches this source, so this rule is not being enforced",
			len(control))
	}
	for _, path := range files {
		if reImportsVue.MatchString(readFile(t, path)) {
			t.Errorf("%s imports vue. That file is plumbing: it must run in a bare console, "+
				"which is what makes it testable without mounting the app. Move the reactive "+
				"part into a composable and leave the mechanism here.", path)
		}
	}
}

// Rule — a composable may not import another composable.
//
// Twelve independent files, each readable alone, is the whole point; a graph of them is
// the thing this replaced. What a composable needs arrives as an argument, and reactive
// inputs it does not own arrive as getters, so it cannot hold a stale array.
func TestComposablesDoNotImportEachOther(t *testing.T) {
	files := filesIn(t, filepath.Join(uiSrc, "composables"), ".js")
	if len(files) < 8 {
		t.Fatalf("only %d composables — has the layer moved?", len(files))
	}
	seen := 0
	for _, path := range files {
		for _, m := range reImport.FindAllStringSubmatch(readFile(t, path), -1) {
			seen++
			if strings.HasPrefix(m[1], "./") && strings.HasSuffix(m[1], ".js") {
				t.Errorf("%s imports %s — a composable may not reach for another composable's "+
					"state. Pass what it needs in (a getter for anything reactive it does not "+
					"own), or the flat set of files becomes a graph.", path, m[1])
			}
		}
	}
	// Every composable imports at least `ref` or `computed` from vue, so zero means the
	// pattern stopped matching the source — not that the tree got clean. That is the failure
	// this assertion exists for: it happened once, when the quote style changed.
	if seen == 0 {
		t.Fatalf("parsed %d import statements across %d composables — reImport no longer "+
			"matches this source, so this rule is not being enforced", seen, len(files))
	}
}

// Rule — a component holds no branches.
//
// A component is a contract: props in, events out, composables behind it, template. The
// moment its script grows an `if`, some piece of logic has no name and no home, and the
// split that made the BA screen 54 lines instead of 179 has started to undo itself.
// Branching in the *template* is fine and expected — a `v-if` asks a question some
// composable already answered.
func TestComponentsHoldNoLogic(t *testing.T) {
	files := filesIn(t, filepath.Join(uiSrc, "components"), ".vue")
	if len(files) < 4 {
		t.Fatalf("only %d components — has the layer moved?", len(files))
	}
	// Both patterns assert an absence, so the same trap as reImportsVue applies: one that
	// matches nothing passes for the wrong reason. App.vue is the positive control for the
	// ternary — it is the shell, which this rule does not govern, and it reads a stored mode
	// with one. If the pattern cannot find that, it is broken rather than satisfied.
	if !reTernary.MatchString(stripComments(scriptOf(t, filepath.Join(uiSrc, "App.vue")))) {
		t.Fatal("reTernary matched nothing in App.vue, which branches on the stored mode — " +
			"the pattern no longer matches this source, so this rule is not being enforced")
	}
	for _, path := range files {
		script := stripComments(scriptOf(t, path))
		if m := reBranch.FindString(script); m != "" {
			t.Errorf("%s: `%s` in <script setup>. A component with a branch is a composable "+
				"nobody wrote yet — move it into one and let the template ask the question.",
				path, strings.TrimSpace(m))
		}
		// The template may branch all it likes — `{{ importing ? 'IMPORTING…' : 'CHOOSE' }}`
		// is how a template asks a question. This is about <script setup> deciding.
		if m := reTernary.FindString(script); m != "" {
			t.Errorf("%s: `%s` in <script setup>. A ternary is still a branch — put the rule "+
				"in the lib/ module or composable that owns it, so it can be read and tested "+
				"without mounting anything.", path, strings.TrimSpace(m))
		}
	}
}

// Rule — the shell is wiring, so it may not reach for transport.
//
// App.vue composes composables and mounts components. A `fetch` reached from the shell is
// the first step back to the 492-line app.js this replaced: state ends up next to
// transport and neither can be read without the other. viewport.js is the one exception —
// it binds to the dock element the shell owns — and it is named here rather than left as
// a surprise.
func TestTheShellDoesNotReachForTransport(t *testing.T) {
	const allowed = "./lib/viewport.js"
	imports := reImport.FindAllStringSubmatch(scriptOf(t, filepath.Join(uiSrc, "App.vue")), -1)
	// The shell composes every composable and mounts every component, so a single-digit
	// count means the pattern stopped matching rather than the shell going quiet.
	if len(imports) < 8 {
		t.Fatalf("parsed only %d imports in App.vue — reImport no longer matches this source, "+
			"so this rule is not being enforced", len(imports))
	}
	for _, m := range imports {
		if strings.HasPrefix(m[1], "./lib/") && m[1] != allowed {
			t.Errorf("App.vue imports %s. The shell wires; transport and rendering belong "+
				"behind a composable or a component. (%s is the one exception: it binds to "+
				"the dock element the shell owns.)", m[1], allowed)
		}
	}
}

/* ── reading the tree ────────────────────────────────────────────────────────── */

func filesIn(t *testing.T, dir, ext string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// scriptOf returns an SFC's <script setup> body, or the whole file for a plain module.
func scriptOf(t *testing.T, path string) string {
	t.Helper()
	src := readFile(t, path)
	if !strings.HasSuffix(path, ".vue") {
		return src
	}
	m := reScript.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("%s has no <script setup> block", path)
	}
	return m[1]
}

// stripComments removes /* … */ and // … , so the word "if" inside a comment — and these
// files carry a lot of prose — cannot read as a branch.
func stripComments(src string) string {
	return reLineComment.ReplaceAllString(reBlockComment.ReplaceAllString(src, ""), "")
}

var (
	// Both quote styles, deliberately. These read JavaScript as *text*, so a formatting
	// choice must not be able to disarm them — and one already did: the day eslint-stylistic
	// started enforcing single quotes, a `"vue"`-only pattern matched nothing and rules 9, 10
	// and 12b passed vacuously while `make check` stayed green. A regex enforcer that finds
	// nothing looks exactly like a codebase with nothing to find, which is why the tests
	// below also assert they parsed something.
	reImportsVue = regexp.MustCompile(`(?m)^\s*import\s+[^;]*from\s+['"]vue['"]`)
	reImport     = regexp.MustCompile(`(?m)^\s*import\s+(?:[^;]*?from\s+)?['"]([^'"]+)['"]`)
	reScript     = regexp.MustCompile(`(?s)<script setup>(.*?)</script>`)
	reBranch     = regexp.MustCompile(`(?m)^\s*(?:if|for|while|switch)\s*\(`)
	// A ternary is a branch that reBranch cannot see: it is an expression, so it never
	// starts a line with a keyword. One escaped for exactly that long — ChatTurn.vue decided
	// two mid-stream rendering rules (`streaming ? [] : citations`) inside <script setup>
	// while rule 11 reported clean. `[^.?]` after the `?` keeps optional chaining (`a?.b`)
	// and nullish coalescing (`a ?? b`) out of it; both are value access, not a decision.
	reTernary      = regexp.MustCompile(`\?[^.?][^:\n]*:`)
	reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reLineComment  = regexp.MustCompile(`(?m)//.*$`)
)
