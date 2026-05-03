.PHONY: help build install install-system uninstall-system test compose-build compose-up compose-down e2e e2e-filter e2e-up e2e-run e2e-rebuild e2e-down dev dev-down clean tag patch minor major undo push

# `make tag {patch|minor|major}`  — bump the latest semver tag and create
#                                   an annotated git tag at HEAD (local
#                                   only; doesn't touch origin).
# `make tag push`                 — push the most recent local tag to
#                                   origin.
# `make tag undo`                 — delete the most recent local tag.
#                                   Remote tags (if any) are out of
#                                   scope — handle those with `git push
#                                   --delete origin <tag>` directly.
#
# The action word is passed as a positional goal; we pluck it from
# MAKECMDGOALS and treat the action-name targets as no-ops so Make
# doesn't try to build them.
TAG_ACTION := $(filter patch minor major undo push,$(MAKECMDGOALS))

# Where `make install-system` copies the user-facing CLI. /usr/local/bin
# is on the default macOS + most Linux PATHs out of the box and isn't
# managed by Homebrew, so dropping a binary in there won't fight with
# `brew`.
SYSTEM_BIN ?= /usr/local/bin

COMPOSE := docker compose -f compose/docker-compose.yml

# Version + SHA injected into binaries via -ldflags. `git describe` falls
# back gracefully on shallow clones (just the SHA). `--dirty` flags when
# the working tree has uncommitted changes.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
SHA     := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X github.com/pipescloud/ppz/internal/version.Version=$(VERSION) \
           -X github.com/pipescloud/ppz/internal/version.BuildSHA=$(SHA)

help:
	@echo "Targets:"
	@echo "  build             Build all four binaries into ./bin/"
	@echo "  install           Install binaries into \$$(go env GOBIN) (or ~/go/bin)"
	@echo "  install-system    Copy ./bin/ppz to \$$(SYSTEM_BIN) (default /usr/local/bin) — requires sudo"
	@echo "  uninstall-system  Remove ppz from \$$(SYSTEM_BIN) — requires sudo"
	@echo "  test              Run Go unit tests"
	@echo "  compose-build     Build all docker images"
	@echo "  compose-up        Start the e2e environment (postgres, server, daemons, GUIs)"
	@echo "  compose-down      Tear down the e2e environment + volumes"
	@echo "  e2e               Run the full bash test suite (one-shot: builds + starts + runs)"
	@echo "  e2e-filter F=…    Run a subset (one-shot, includes build + start)"
	@echo ""
	@echo "Fast iteration mode (skip rebuild between runs):"
	@echo "  e2e-up            Build images + start the stack (run once per session)"
	@echo "  e2e-run F=…       Run scenarios against an already-up stack (~5-10s)"
	@echo "  e2e-rebuild       Rebuild images + recreate containers (after Go code changes)"
	@echo ""
	@echo "Local-OAuth manual-test rig (real github.com via compose overlay):"
	@echo "  dev               Bring up postgres + ppz-server pointed at real github.com, install CLI, restart daemon"
	@echo "  dev-down          Tear down the dev compose stack + daemon"
	@echo "  e2e-down          Tear down the stack and volumes"
	@echo "  tag {patch|minor|major}"
	@echo "                    Bump the requested semver component from the latest tag and"
	@echo "                    create an annotated git tag at HEAD. Local only — push"
	@echo "                    explicitly with: make tag push"
	@echo "  tag push          Push the most recent local tag to origin."
	@echo "  tag undo          Delete the most recent local tag. Remote tags are out of"
	@echo "                    scope — remove with: git push --delete origin <tag>"

build:
	@mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ppz           ./cmd/ppz
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ppz-server    ./cmd/ppz-server
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ppz-desktop   ./cmd/ppz-desktop
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ppz-seed      ./cmd/ppz-seed
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ppz-natsbootstrap ./cmd/ppz-natsbootstrap
	@echo "Built $(VERSION) ($(SHA)) into ./bin/:"
	@ls -1 bin/

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/ppz ./cmd/ppz-server ./cmd/ppz-desktop ./cmd/ppz-seed ./cmd/ppz-natsbootstrap
	@dest=$$(go env GOBIN); [ -n "$$dest" ] || dest="$$(go env GOPATH)/bin"; \
		echo "Installed $(VERSION) ($(SHA)) into $$dest/"; \
		echo "(if 'ppz' is not on your PATH, add: export PATH=\"$$dest:\$$PATH\" to your shell rc, or use 'make install-system')"

# Build + drop the user-facing CLI into a system PATH location. Only
# installs `ppz` itself — `ppz-server` / `ppz-desktop` / `ppz-seed` are
# operator/dev tools, not user-facing, so they stay out of /usr/local/bin.
install-system: build
	@if [ ! -w "$(SYSTEM_BIN)" ]; then \
		echo "Installing into $(SYSTEM_BIN) (requires sudo)…"; \
		sudo install -m 0755 ./bin/ppz "$(SYSTEM_BIN)/ppz"; \
	else \
		install -m 0755 ./bin/ppz "$(SYSTEM_BIN)/ppz"; \
	fi
	@echo "Installed $(VERSION) ($(SHA)) → $(SYSTEM_BIN)/ppz"
	@resolved=$$(command -v ppz 2>/dev/null || true); \
	if [ -n "$$resolved" ] && [ "$$resolved" != "$(SYSTEM_BIN)/ppz" ]; then \
		echo "WARNING: 'ppz' resolves to $$resolved before $(SYSTEM_BIN)/ppz on PATH"; \
		echo "         install there instead with: make install-system SYSTEM_BIN=$$(dirname "$$resolved")"; \
	fi
	@# Tab completion: append a one-line eval to the user's shell rc so
	@# new shells pick up `ppz` completion automatically. Idempotent —
	@# guarded by a marker so re-running install-system is safe. The
	@# eval re-runs `ppz completion <shell>` on every shell start, so
	@# adding new verbs or changing the completion script doesn't need
	@# re-installation; only the binary does.
	@case "$$SHELL" in \
		*/zsh)  shell=zsh;  rc="$$HOME/.zshrc";  ;; \
		*/bash) shell=bash; rc="$$HOME/.bashrc"; ;; \
		*) echo "(skipping completion: \$$SHELL=$$SHELL not zsh/bash)"; exit 0 ;; \
	esac; \
	marker='# ppz: shell completion (added by make install-system)'; \
	if [ -f "$$rc" ] && grep -Fq "$$marker" "$$rc"; then \
		echo "Shell completion already enabled in $$rc"; \
	else \
		printf '\n%s\neval "$$(ppz completion %s)"\n' "$$marker" "$$shell" >> "$$rc"; \
		echo "Shell completion enabled in $$rc"; \
		echo "Open a new shell or run: source $$rc"; \
	fi
	@echo "Verify with: ppz version"

uninstall-system:
	@if [ ! -e "$(SYSTEM_BIN)/ppz" ]; then \
		echo "Nothing to uninstall ($(SYSTEM_BIN)/ppz not found)."; \
	elif [ ! -w "$(SYSTEM_BIN)" ]; then \
		echo "Removing $(SYSTEM_BIN)/ppz (requires sudo)…"; \
		sudo rm -f "$(SYSTEM_BIN)/ppz"; \
	else \
		rm -f "$(SYSTEM_BIN)/ppz"; \
	fi
	@# Strip the completion eval from the user's rc file. Removes both
	@# the marker comment and the next non-blank line (the eval).
	@case "$$SHELL" in \
		*/zsh)  rc="$$HOME/.zshrc";  ;; \
		*/bash) rc="$$HOME/.bashrc"; ;; \
		*) exit 0 ;; \
	esac; \
	marker='# ppz: shell completion (added by make install-system)'; \
	if [ -f "$$rc" ] && grep -Fq "$$marker" "$$rc"; then \
		tmp="$$rc.ppz-uninstall.tmp"; \
		awk -v m="$$marker" '\
			$$0 == m {skip=1; next} \
			skip > 0 {skip--; next} \
			{print}' "$$rc" > "$$tmp" && mv "$$tmp" "$$rc"; \
		echo "Removed completion line from $$rc"; \
	fi

test:
	go test -timeout 30s ./...

compose-build:
	$(COMPOSE) build

compose-up:
	$(COMPOSE) up -d --build

compose-down:
	$(COMPOSE) down -v --remove-orphans

e2e:
	$(COMPOSE) build test-runner
	$(COMPOSE) up -d --build postgres ppz-server daemon-a daemon-b desktop-gui-a desktop-gui-b
	$(COMPOSE) run --rm test-runner timeout 15m bash /tests/run.sh

e2e-filter:
	$(COMPOSE) build test-runner
	$(COMPOSE) up -d --build postgres ppz-server daemon-a daemon-b desktop-gui-a desktop-gui-b
	$(COMPOSE) run --rm -e PPZ_TEST_FILTER='$(F)' test-runner timeout 15m bash /tests/run.sh

# Fast iteration mode. e2e-up prepares the stack once; e2e-run executes the
# bash harness against it without rebuilding or recreating containers — drops
# per-run cost from ~60-90s to ~5-10s. After Go code changes, run e2e-rebuild
# (or just e2e-up again) to refresh the daemon/server images. tests/ is bind-
# mounted into the test-runner, so test-script edits don't need a rebuild.
e2e-up:
	$(COMPOSE) build test-runner
	$(COMPOSE) up -d --build postgres ppz-server daemon-a daemon-b desktop-gui-a desktop-gui-b

e2e-run:
	$(COMPOSE) run --rm -e PPZ_TEST_FILTER='$(F)' test-runner timeout 15m bash /tests/run.sh

e2e-rebuild:
	$(COMPOSE) build
	$(COMPOSE) up -d --build --force-recreate postgres ppz-server daemon-a daemon-b desktop-gui-a desktop-gui-b

e2e-down:
	$(COMPOSE) down -v --remove-orphans

# `make dev` — local-OAuth manual-test rig. Stands up the same compose
# postgres + ppz-server we use for e2e, but overlaid with
# docker-compose.dev.yml so ppz-server talks to real github.com instead
# of mock-github. Mock-github isn't started (the overlay drops the
# depends_on). The server runs in the background; tail logs with
#   docker compose -f compose/docker-compose.yml -f compose/docker-compose.dev.yml logs -f ppz-server
#
# In any terminal:  ppz login http://localhost:8080
# When done:        make dev-down
DEV_COMPOSE := docker compose --env-file .env.local -f compose/docker-compose.yml -f compose/docker-compose.dev.yml

dev:
	@if [ ! -f .env.local ]; then \
	    echo "ERROR: .env.local missing. Copy .env.local.example and fill in your GitHub OAuth values."; \
	    exit 1; \
	fi
	$(DEV_COMPOSE) up -d --build ppz-server
	$(MAKE) install-system
	@ppz daemon stop 2>/dev/null || true
	@ppz daemon start
	@echo ""
	@echo "ready: ppz login http://localhost:8080"
	@echo "logs:  $(DEV_COMPOSE) logs -f ppz-server"

dev-down:
	$(DEV_COMPOSE) down
	@ppz daemon stop 2>/dev/null || true

clean:
	rm -rf bin/

# `tag`'s recipe is one big shell invocation (lines joined with `\`) so
# `exit` short-circuits cleanly between branches. An earlier version
# split into separate `@if` blocks; each runs in its own shell, so the
# undo branch's `exit 0` only exited that block and then Make ran the
# bump path too.
tag:
	@if [ "$(words $(TAG_ACTION))" -ne "1" ]; then \
		echo "usage: make tag {patch|minor|major|push|undo}"; \
		exit 2; \
	fi; \
	if [ "$(TAG_ACTION)" = "undo" ]; then \
		LATEST=$$(git describe --tags --abbrev=0 2>/dev/null); \
		if [ -z "$$LATEST" ]; then \
			echo "no tags to undo"; exit 2; \
		fi; \
		git tag -d "$$LATEST" >/dev/null; \
		echo "Deleted local tag $$LATEST."; \
		exit 0; \
	fi; \
	if [ "$(TAG_ACTION)" = "push" ]; then \
		LATEST=$$(git describe --tags --abbrev=0 2>/dev/null); \
		if [ -z "$$LATEST" ]; then \
			echo "no tags to push"; exit 2; \
		fi; \
		git push origin "$$LATEST"; \
		exit 0; \
	fi; \
	LATEST=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	BARE=$${LATEST#v}; \
	MAJ=$$(echo "$$BARE" | cut -d. -f1); \
	MIN=$$(echo "$$BARE" | cut -d. -f2); \
	PAT=$$(echo "$$BARE" | cut -d. -f3); \
	case "$(TAG_ACTION)" in \
		major) MAJ=$$((MAJ + 1)); MIN=0; PAT=0 ;; \
		minor) MIN=$$((MIN + 1)); PAT=0 ;; \
		patch) PAT=$$((PAT + 1)) ;; \
	esac; \
	NEW="v$$MAJ.$$MIN.$$PAT"; \
	if git rev-parse "$$NEW" >/dev/null 2>&1; then \
		echo "tag $$NEW already exists; aborting"; \
		exit 2; \
	fi; \
	if ! git diff --quiet HEAD; then \
		echo "warning: working tree is dirty — the tag will point at HEAD, not the dirty state."; \
	fi; \
	git tag -a "$$NEW" -m "$$NEW"; \
	echo "Tagged $$NEW (was $$LATEST). Push with: make tag push"

# Positional action goals are no-ops by themselves — they only carry
# data into the `tag` target via MAKECMDGOALS.
patch minor major undo push:
	@:
