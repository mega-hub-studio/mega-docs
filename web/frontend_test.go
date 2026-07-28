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
// Eleven independent files, each readable alone, is the whole point; a graph of them is
// the thing this replaced. What a composable needs arrives as an argument, and reactive
// inputs it does not own arrive as getters, so it cannot hold a stale array.
func TestComposablesDoNotImportEachOther(t *testing.T) {
	files := filesIn(t, filepath.Join(uiSrc, "composables"), ".js")
	if len(files) < 8 {
		t.Fatalf("only %d composables — has the layer moved?", len(files))
	}
	for _, path := range files {
		for _, m := range reImport.FindAllStringSubmatch(readFile(t, path), -1) {
			if strings.HasPrefix(m[1], "./") && strings.HasSuffix(m[1], ".js") {
				t.Errorf("%s imports %s — a composable may not reach for another composable's "+
					"state. Pass what it needs in (a getter for anything reactive it does not "+
					"own), or the flat set of files becomes a graph.", path, m[1])
			}
		}
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
	for _, path := range files {
		if m := reBranch.FindString(stripComments(scriptOf(t, path))); m != "" {
			t.Errorf("%s: `%s` in <script setup>. A component with a branch is a composable "+
				"nobody wrote yet — move it into one and let the template ask the question.",
				path, strings.TrimSpace(m))
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
	for _, m := range reImport.FindAllStringSubmatch(scriptOf(t, filepath.Join(uiSrc, "App.vue")), -1) {
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
	reImportsVue   = regexp.MustCompile(`(?m)^\s*import\s+[^;]*from\s+"vue"`)
	reImport       = regexp.MustCompile(`(?m)^\s*import\s+(?:[^;]*?from\s+)?"([^"]+)"`)
	reScript       = regexp.MustCompile(`(?s)<script setup>(.*?)</script>`)
	reBranch       = regexp.MustCompile(`(?m)^\s*(?:if|for|while|switch)\s*\(`)
	reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reLineComment  = regexp.MustCompile(`(?m)//.*$`)
)
