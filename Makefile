# sqlite-vec (cgo) + FTS5 require CGO + these build tags.
TAGS := sqlite_fts5
export CGO_ENABLED := 1

# The Go packages, spelled out rather than `./...`.
#
# web/ui is a Vite project, and npm installs whatever its dependencies ship — one of them
# (flatted) ships a Go package. `./...` walks into web/ui/node_modules and hands the
# linter somebody else's code: seven findings that are not this repository's, in a gate
# whose whole value is that a finding means something. The Go tool has no ignore for
# directories, so the pattern is the fix.
PKGS := ./cmd/... ./internal/... ./web

# The deploy host's three facts, all overridable (`make deploy UNIT=… PORT=… DEPLOY_DIR=…`).
# PORT is read from .env rather than repeated here, because .env is what the server itself
# reads — a port written twice is a health check that passes against nothing on the day one of
# them changes.
#
# DEPLOY_DIR is the checkout the supervisor runs from, and it exists because the supervisor
# names that absolutely (`ExecStart=/opt/knowledge/bin/knowledge`) while `make` is relative to
# wherever it was typed. Without it, `deploy` from a dev tree succeeds against the wrong
# directory at every step: it pulls *here*, builds `./bin/knowledge` *here*, restarts the unit
# — which re-execs the other binary, untouched — then prints a revision and a green health
# check. A no-op that reports success, which is the one outcome the guards on `deploy-here`
# exist to prevent. On macOS the checkout is wherever the launchd plist points; override it.
UNIT ?= knowledge
DEPLOY_DIR ?= /opt/knowledge
PORT ?= $(shell sed -n 's/^PORT=//p' .env 2>/dev/null | tail -1)
HEALTH := http://127.0.0.1:$(or $(PORT),8080)/api/health

# The supervisor is the only part of a deploy that is not portable: the binary, the .env, the
# health check and the way it is published are the same on both. So it is two variables and
# not a second target — a macOS copy of `deploy` would carry its own drifting version of the
# four guards documented above it. Before this, `make deploy` on a Mac built the new binary
# and then died on `systemctl`, leaving the old process serving and the log reading like a
# deploy had happened.
#
# `kickstart -k` restarts an already-bootstrapped job and fails if there is none, which is
# what `systemctl restart` does with an unknown unit — a missing agent must not look like a
# successful deploy. Installing it is a first-install step, on the Deploy page.
ifeq ($(shell uname -s),Darwin)
LABEL  ?= dev.megadocs.knowledge
RESTART := launchctl kickstart -k gui/$(shell id -u)/$(LABEL)
STATUS  := launchctl print gui/$(shell id -u)/$(LABEL)
KNOWN   := launchctl print gui/$(shell id -u)/$(LABEL) >/dev/null 2>&1
NAMED   := LABEL=$(LABEL)
else
RESTART := sudo systemctl restart $(UNIT)
STATUS  := systemctl status --no-pager -n 20 $(UNIT)
KNOWN   := systemctl cat $(UNIT) >/dev/null 2>&1
NAMED   := UNIT=$(UNIT)
endif

# Where `go install` puts a tool, which is where the targets that install one look for it.
# GOBIN wins when it is set, or `dead-deps` installs on every run and finds nothing.
GOBIN_DIR := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

.PHONY: deps check check-full ui-deps test lint lint-deps lint-fix lint-js dead dead-deps secrets live smoke server ingest build vendor vendor-clean diagram clean check-ui check-wt ui ui-dev deploy deploy-here release backup

deps:
	go mod tidy

# Everything CI should gate on: vet, tests, linters.
#
# gofmt is deliberately *not* a step of its own. It is a formatter in .golangci.yml, so
# `lint` already reports it — and reports it over $(PKGS) instead of over `.`, which is
# the difference that matters: `gofmt -l .` walked web/ui/node_modules, where one npm
# dependency ships a Go package. That is the same trap $(PKGS) exists to close, reopened
# by a shorter command. It had not fired yet only because that file happens to be
# formatted.
check: test secrets
	go vet -tags "$(TAGS)" $(PKGS)
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory lint-js
	@$(MAKE) --no-print-directory dead

# ── THE FINAL GATE: one command to run after implementing anything ──
# `check` is what CI gates on and what you run while working. This is the superset you
# run *before* saying it is done, and the order is the point: each stage is cheaper than
# the next, so the first thing to break is the first thing you hear about.
#
#   1. ui        rebuild the bundle FIRST — TestBuiltUIMatchesItsSources (inside `check`)
#                compares web/dist against a hash of its sources, so running `check`
#                before this reports a stale bundle rather than the bug you introduced.
#   2. check     vet · every Go test · golangci-lint (gofmt included) · deadcode
#                · credential scan
#                · eslint (which is also the formatter now: style rules are errors, so a
#                missing reformat fails here rather than in review)
#   3. build     both binaries, because `go build` catches what `go vet` does not
#   4. check-ui  the guide rendered, served and measured in Chromium, both languages
#   5. check-wt  every diagram walkthrough driven prev/next at two viewports
#
# 4 and 5 need node + pinchtab + a browser, and *skip* (0) without them, so this target
# stays runnable on a box that has none — read the "skipped" lines rather than assuming a
# green run covered them. That warning is not theoretical: both skipped on every machine
# for as long as they gated on a hardcoded Playwright path.
#
# Two stages that used to skip no longer can, because their tool needs nothing this gate
# does not already have: `dead` installs deadcode (Go), `lint-js` is the one left, and it
# skips only when web/ui/node_modules is absent — which `make ui`, stage 1, has just made
# sure it is not.
check-full:
	@$(MAKE) --no-print-directory ui
	@$(MAKE) --no-print-directory check
	@$(MAKE) --no-print-directory build
	@$(MAKE) --no-print-directory check-ui
	@$(MAKE) --no-print-directory check-wt
	@echo ""
	@echo "  check-full: PASS — bundle fresh, Go + JS clean, guide and walkthroughs measured"

# ── golangci-lint, pinned ─────────────────────────────────────────────────────
# One version for this repo, and CI reads this same variable — so there is no second
# place for it to be true.
#
# Pinned rather than @latest because @latest is a gate that moves on somebody else's
# schedule: a new linter or a new rule in an upstream release turns a green tree red
# without a commit. Not theoretical — it happened here on `goconst` and a new `gosec`
# rule, and the older binary left on PATH locally reported zero while CI failed. The old
# fix was to *print* the version and ask a human to compare it with check.yml by eye,
# which is a symptom fix: the versions could still differ, you just got to read about it.
#
# `lint-deps` installs the pinned one when the binary is missing *or* is a different
# version, which is the same move `ui-deps` makes for node_modules and for the same
# reason — nobody wants "go install the other version" from a target whose whole job is
# to lint. Local and CI now agree by construction rather than by inspection.
#
# It is the binary install because that is what upstream recommends over `go install`,
# which "isn't guaranteed to work": the result depends on the local Go version, `replace`
# directives do not apply transitively, and tool dependencies can collide with the
# project's own.
GOLANGCI_VERSION := v2.12.2
GOLANGCI_BIN := $(GOBIN_DIR)/golangci-lint

lint-deps:
	@$(GOLANGCI_BIN) version 2>/dev/null | grep -qF " $(patsubst v%,%,$(GOLANGCI_VERSION)) " || { \
		echo "  installing golangci-lint $(GOLANGCI_VERSION)"; \
		curl -sSfL https://golangci-lint.run/install.sh \
			| sh -s -- -b "$(dir $(GOLANGCI_BIN))" $(GOLANGCI_VERSION); }

# Configured by .golangci.yml — which explains every linter it leaves off, because the
# stock config reports 591 issues on this tree and a gate that always shouts is a gate
# nobody reads. It currently reports zero; a new finding means a new fact. staticcheck
# runs inside it, which is why `dead` no longer runs it separately.
#
# A *warning* fails too. golangci-lint exits zero on `warn-unused` — the message that an
# exclusion rule in .golangci.yml no longer matches anything — and an exclusion nobody
# needs is exactly the dead config rule 24 calls a lie in the gate. Left as a warning it
# would scroll past on every green run, which is how it stays forever.
lint: lint-deps
	@out=$$($(GOLANGCI_BIN) run $(PKGS) 2>&1); code=$$?; \
	printf '%s\n' "$$out"; \
	[ $$code -eq 0 ] || exit $$code; \
	case "$$out" in *level=warning*) \
		echo "^ an exclusion rule in .golangci.yml matched nothing — delete it or fix its path"; \
		exit 1 ;; \
	esac

# ── the app's front end ───────────────────────────────────────────────────────
# web/ui is a Vite project (Vue 3.5 SFCs, JavaScript). Its output, web/dist, is committed
# and embedded by the binary — so `go build`, `go install` and `git pull && make build` on
# the host all keep working without Node. That is the whole reason a build artefact is in
# git, and TestBuiltUIMatchesItsSources is what stops it going stale.
# node_modules has to match the lockfile, and "it exists" is not the same question.
# `[ -d node_modules ] || npm ci` was the old test, so a `git pull` that *added* a
# dependency left a directory that existed and was stale — and the failure surfaced as a
# Vite overlay saying `Failed to resolve import "vue-i18n" from "src/main.js"`, pointing at
# your own source, which is the least useful place to look. That happened, on the commit
# that added vue-i18n.
#
# npm writes node_modules/.package-lock.json on every install, so a lockfile newer than that
# stamp means the tree is behind. Installing is the right move rather than only reporting:
# nobody wants "run npm ci" from a target whose whole job is to build the front end.
UI_STAMP := web/ui/node_modules/.package-lock.json
ui-deps:
	@if [ ! -f $(UI_STAMP) ] || [ web/ui/package-lock.json -nt $(UI_STAMP) ]; then \
		echo "  web/ui/node_modules is missing or behind package-lock.json — installing"; \
		cd web/ui && npm ci --no-audit --no-fund; \
	fi

ui: ui-deps
	@cd web/ui && npm run build

# HMR against the real engine: serves the UI on :5179 and proxies /api to :8080, so run
# `make server` in another shell first.
ui-dev: ui-deps
	@cd web/ui && npm run dev

# The walkthroughs, driven. Separate from check-ui because it answers a different
# question: check-ui measures layout, this one measures behaviour — prev/next, the dots,
# the keyboard, and whether each step's `data-focus` still matches a node in its diagram.
# A renamed node lights nothing, which no layout measurement would notice.
check-wt:
	@./scripts/check-walkthroughs.sh

# The browser check: what a screenshot shows, measured. Not in `check` — it needs a
# browser, and this product needs no Node at all (see CLAUDE.md rule 14). Run it after
# touching a docs page, a recipe or docsbase.html.
#
# Both browser checks are driven by PinchTab (`npm i -g pinchtab`, then `pinchtab doctor`),
# which replaced Playwright. The old gate was a hardcoded /opt/node22/... path, so on any
# machine without it these skipped — and a skipped check reads exactly like a passing one.
# One assertion did not survive the move: touch-target size, which needs a coarse-pointer
# emulation PinchTab does not have. It is named at the top of scripts/check-docs-ui.mjs.
check-ui:
	@./scripts/check-docs-ui.sh

# ESLint over web/ui — and it is in `check`, unlike before. What changed is that there are
# .vue files now: eslint-plugin-vue's `vue/no-undef-properties` reads a real parse of a
# real component and catches a template binding with nothing behind it, which is the rule a
# Go regex used to approximate. It runs from the project's own devDependencies (installed
# by `make ui`), so there is no second eslint cache to keep in step.
#
# Skipped, loudly, when node_modules is absent: a machine that only builds Go still has a
# committed bundle and does not need a linter for it.
#
# CI=1 for the same reason `lint-deps` pins a version: local and CI have to agree by
# construction. @antfu/eslint-config turns rules off when it sees an editor's environment
# (VSCODE_PID and friends) unless CI is set, so a terminal *inside* the IDE ran a weaker
# gate than the push did — and that is not theoretical, it hid a `jsdoc/*` warning that
# --max-warnings 0 fails on.
# The guard and the command are ONE recipe line, and that is the whole point: make runs every
# line in its own shell, so an `exit 0` in a guard on the line above ends *that* shell and make
# cheerfully runs the next one. Written as two lines — which it was — a box without Node printed
# "skipped lint-js" and then failed `eslint: not found` with exit 127, so the skip this repo
# documents had never once happened. Measured by moving node_modules aside.
lint-js:
	@if [ -d web/ui/node_modules ]; then cd web/ui && CI=1 npm run --silent lint; \
	else echo "  skipped lint-js (run \`make ui\` to install web/ui)"; fi


# Same linters as the gate, applying the fixes they know how to make — *both* languages, the
# way `check` runs `lint` and `lint-js` rather than one of them. It was Go-only until then,
# which made "the formatter is in the gate" true and "one command formats this repo" false:
# `npm run lint:fix` existed and nothing called it, so the JS half was a thing you had to
# remember. One door, both sides.
#
# Read the diff: the formatters are opinionated and one of them (gofumpt) is left off here for
# a reason .golangci.yml spells out. On the JS side the fixer is ESLint's own — eslint-stylistic
# for semicolons, quotes and trailing commas, never Prettier, and it reflows nothing.
#
# Deliberately NOT part of `check`: a gate that rewrites files reports green on code nobody
# reviewed, and formatting is already *verified* there (gofmt as a golangci-lint formatter,
# ESLint's style rules as errors). `gofmt -l .` was deleted from `check` for that first reason
# and for walking web/ui/node_modules; this is the fixer, not a second checker.
# The JS half is inline rather than its own target: it would have had exactly one caller, and a
# comment saying "nothing else should call this" is an admission that the name earns nothing.
# `npm run lint:fix` is the one-liner for anybody who wants only that half.
lint-fix: lint-deps
	$(GOLANGCI_BIN) run --fix $(PKGS)
	@if [ -d web/ui/node_modules ]; then cd web/ui && npm run --silent lint:fix; \
	else echo "  skipped the JS half of lint-fix (run \`make ui\` to install web/ui)"; fi

# What no linter finds: a function no binary can reach. staticcheck's unused only sees
# within a package, and it now runs inside `lint` anyway; deadcode does whole-program
# reachability from the two mains. A *finding* must fail the build, which is why this is
# not written as `tool || echo` (that turns a non-zero exit into a cheerful message).
#
# Installed rather than skipped, which is the move `lint-deps` and `ui-deps` already make —
# and the case for it is stronger here: the only thing this install needs is the Go
# toolchain, which every box running `make check` has by definition. So the old skip
# protected nobody and cost rule 17 its enforcer, printing "skipped deadcode (go install
# …)" on every run of a developer machine's gate. A skipped check reads exactly like a
# passing one; that is the same trap the hardcoded Playwright path set, and it was open
# here the whole time. CI installs nothing of its own now either — one installer for this
# tool, the way there is one for the linter.
#
# @latest, unlike GOLANGCI_VERSION, because there is no rule set to move under us: it
# either finds code unreachable from a main or it does not.
DEADCODE_BIN := $(GOBIN_DIR)/deadcode

dead-deps:
	@[ -x "$(DEADCODE_BIN)" ] || { echo "  installing deadcode"; \
		go install golang.org/x/tools/cmd/deadcode@latest; }

dead: dead-deps
	@out=$$($(DEADCODE_BIN) -tags "$(TAGS)" ./cmd/...); \
	if [ -n "$$out" ]; then echo "$$out"; echo "^ unreachable from any binary"; exit 1; fi

# Nothing key-shaped may be committed. .env is gitignored; this catches the case
# where a key gets pasted into a tracked file by accident.
# Two of the exclusions are generated files whose content is hashes by definition:
# web/dist (Vite's bundle, where minified third-party code matches by accident) and
# package-lock.json (every entry carries a sha512 integrity string). Excluding the *build
# output* is not a blind spot — a key can only reach the bundle from web/ui/src, which is
# scanned.
#
# A third, web/ui/pnpm-lock.yaml, fails the same way and is *deleted and gitignored* rather
# than excluded — .gitignore says why. This scan is how it announced itself both times it was
# committed, so leaving it unexcluded is the guard, not an oversight.
#
# git grep only sees TRACKED files, which is worth knowing before trusting a green run:
# running this before `git add` scans a smaller tree than CI does. That is exactly how the
# lockfile got past a local `make check` and failed on the first push.
secrets:
	@! git grep -nIE '(sk|api|key|token)[-_]?[A-Za-z0-9]{24,}' -- . \
		':!*.sha384' ':!scripts/*' ':!web/dist/*' \
		':!web/ui/package-lock.json' \
		|| { echo "^ that looks like a credential in a tracked file"; exit 1; }

test:
	go test -tags "$(TAGS)" $(PKGS)

# Run the chat server (http://localhost:8080)
server:
	go run -tags "$(TAGS)" ./cmd/server

# Index docs:  make ingest DOCS=./docs
ingest:
	go run -tags "$(TAGS)" ./cmd/ingest $(DOCS)

# Compile the two binaries. web/vendor/ is not part of either: only web/vendor.sha384 is
# embedded (web/assets.go), and the tree itself is read from disk by tooling — `rendocs -base
# /vendor` and `make check-ui`. So nothing here needs `make vendor` first, and a CI job that
# runs one before a build pays for an asset fetch the binary never sees.
build:
	go build -tags "$(TAGS)" -o bin/knowledge ./cmd/server
	go build -tags "$(TAGS)" -o bin/ingest   ./cmd/ingest

# Cut a release: the annotated tag is the only input, and web/release.json is generated
# from git log in the commit the tag points at. Not part of `build` or `deploy` — cutting a
# release is a decision somebody makes, and a tag is a side effect no automated target may
# take. scripts/release.sh explains the write-commit-tag order.
release:
	@V="$(V)" scripts/release.sh

# `make deploy` from any tree on the machine. All it decides is *where* the work happens; the
# work is `deploy-here`, which assumes it is already in the checkout the supervisor executes
# from. That split is the whole fix for the failure DEPLOY_DIR describes above — the wrong
# directory is no longer reachable, rather than merely documented as "cd there first".
#
# It hands over to `deploy` and not to `deploy-here`: a deploy checkout still carrying an older
# Makefile has no `deploy-here`, and its own `deploy` is correct in place — so the first run
# after this change installs the dispatcher instead of failing on it. Once installed, that copy
# sees itself and runs the work directly.
#
# The unpushed-commit note is the other half of "it did nothing": this target moves the host to
# what origin already has (below), so work that never left this tree is invisible to it. Saying
# so beats a green deploy that shipped none of it. A note, not a refusal — redeploying or
# rolling back to what origin has is a legitimate reason to run this.
deploy:
	@here=$$(pwd -P); \
	there=$$(cd "$(DEPLOY_DIR)" 2>/dev/null && pwd -P) || { \
	  echo "  refusing: DEPLOY_DIR=$(DEPLOY_DIR) is not a directory — pass DEPLOY_DIR=…"; exit 1; }; \
	if [ "$$here" = "$$there" ]; then \
	  $(MAKE) --no-print-directory deploy-here; \
	else \
	  [ -d "$$there/.git" ] || { \
	    echo "  refusing: $$there is not a git checkout — pass DEPLOY_DIR=…"; exit 1; }; \
	  n=$$(git rev-list --count '@{u}..HEAD' 2>/dev/null) || n=; \
	  if [ -n "$$n" ] && [ "$$n" != 0 ]; then \
	    echo "  note: $$n commit(s) here are not pushed — the deploy cannot see them"; \
	  fi; \
	  echo "  deploying $$there"; \
	  $(MAKE) -C "$$there" --no-print-directory deploy; \
	fi

# Re-deploy the checkout this runs in: pull, build, restart, prove it came back. Four things it
# does that the hand-typed `git pull && make build && sudo systemctl restart knowledge` does
# not, each one a failure that has already happened here:
#
#   the unit    asked *first*, because a supervisor that has never heard of it is the one
#               failure that leaves the deploy half-done: a typo'd name (`knowledgey`) got all
#               the way through pull and build, so the new binary replaced the old one on disk
#               while the running process kept serving the deleted inode — 31 minutes stale,
#               `ok:true`, and the health check never reached. `make deploy` failed loudly and
#               was still the wrong shape: refusing costs nothing and changes nothing.
#   --ff-only   a deploy checkout is a mirror, not a branch. `pull.rebase=true` is set on
#               this host, so a local commit turned an upgrade into a half-finished rebase
#               with a conflicted lockfile — mid-deploy, on the machine serving the team.
#               --ff-only refuses instead, and says so while the old binary is still running.
#   stale UI    web/dist is committed and embedded, and a deploy host may have no Node to
#               rebuild it. A push that forgot `make ui` deploys a binary whose UI predates
#               the change, which looks like the deploy silently did nothing. That test names it.
#   revision    printed before and after, so the log line answers "did this change anything?"
#   health      a restart that fails leaves the supervisor retrying and the old answer cached
#               in somebody's browser. Not verifying is how a broken deploy stays quiet.
#
# Deliberately not here: `make ui`, `make check-full` (Node, and a browser, neither of which
# a deploy host has) and any `git push`. This target only moves this machine to what origin
# already has.
deploy-here:
	@$(KNOWN) || { echo "  refusing: this machine has no $(NAMED) — nothing would be restarted"; exit 1; }
	@git diff --quiet || { echo "  refusing: working tree is dirty — commit or stash first"; git status --short; exit 1; }
	@echo "  before: $$(git rev-parse --short HEAD)"
	git pull --ff-only
	@go test -tags "$(TAGS)" -count=1 -run TestBuiltUIMatchesItsSources ./web/ \
		|| { echo "  refusing: web/dist is stale — run \`make ui\` where Node is, commit it, push"; exit 1; }
	@$(MAKE) --no-print-directory build
	$(RESTART)
	@for i in $$(seq 1 20); do \
		curl -sf -o /dev/null "$(HEALTH)" && break || sleep 0.5; \
	done; \
	curl -sf "$(HEALTH)" | grep -q '"ok":true' \
		|| { echo "  FAILED: $(UNIT) did not answer $(HEALTH) after the restart"; \
		     $(STATUS); exit 1; }
	@echo "  deployed: $$(git rev-parse --short HEAD) — $$(curl -s $(HEALTH))"
# A note, not a refusal, and the same shape as the unpushed-commits one above: shipping
# unreleased work is normal, shipping it without ever noticing is what went wrong. The badge
# in the app names the last tag, so every commit past it is a change the modal cannot show —
# and the one moment an operator is guaranteed to be looking at this is right after the health
# line prints "release":"v0.13.0" beside a commit that is forty ahead of it. That is exactly
# how this tree ended up with 40 unreleased commits and a v0.13.0 badge nothing backed: rule 25
# defeated the VERSION file, then re-created it one level up as a tag somebody has to remember.
	@t=$$(git describe --tags --abbrev=0 2>/dev/null) || t=; \
	if [ -n "$$t" ]; then \
	  n=$$(git rev-list --no-merges --count "$$t..HEAD"); \
	  [ "$$n" = 0 ] || echo "  note: $$n commit(s) since $$t — the badge still names $$t; \`make release V=…\` is what updates the modal"; \
	fi

# One snapshot of DB_PATH, verified, outside this machine's disk. The same script the nightly
# timer runs (Deploy page), so the hand-run path and the scheduled one cannot drift — and
# `make backup DEST=…` is what you type before a migration or anything else irreversible.
# It does not stop the service: read scripts/backup.sh for why that is safe.
backup:
	@scripts/backup.sh

# Probe a real provider: does it have both endpoints, what embedding width, does
# chat stream? Skipped unless AI_API_KEY is set (read from .env).
live:
	go test -tags "$(TAGS) live" -v -count=1 -run TestLive ./internal/ai/

# Full round trip against a real provider: ingest a fixture, ask, verify the
# answer streams and cites it.
smoke:
	sh scripts/smoke.sh

# Download + digest-verify the *docs pages'* CDN assets into web/vendor/, so the guide can
# be rendered and read with no egress: `rendocs -base /vendor`. Not the app — the app's
# dependencies are bundled into web/dist by Vite, and there is no ASSET_BASE any more.
# Pins live in web/vendor.sha384.
vendor:
	sh scripts/vendor.sh

vendor-clean:
	find web/vendor -mindepth 1 ! -name .gitkeep -delete

# Render web/*.mmd to web/*.svg. Only needed after editing a diagram — the SVG is
# committed, so a normal build and CI never run this and never need mermaid.
# Requires the vendored assets (for the design system's own theme + fonts) and
# fetches mermaid into .cache/ as a build-time-only tool.
diagram: vendor
	node scripts/gen-diagram.mjs

clean:
	rm -rf bin knowledge.db knowledge.db-*
