# Generic Linux Makefile for building GO programs for multiple platforms
# @(#) $Id: Makefile,v 1.10 2026/11/04 12:00:00 ralph Exp $
# Loc: $Header: /home/cvs/src/go/Makefile,v 1.10 2026/11/04 12:00:00 ralph Exp $
#
# Parallelism: every platform target is independent -- each writes its own
# file under $(OUTPUT_DIR), and $(OUTPUT_DIR) is an order-only prerequisite
# created once before any build starts. So `make -j` is safe and scales
# near-linearly: `make -j$(nproc)` builds all 8 platforms in roughly the
# time of the slowest single one. Do NOT use -j with release targets:
# settag and pushtag must run sequentially in that order.

# Program name is derived from the directory name (tree.go in dir tree/ -> tree).
PROGRAM = $(shell basename $(CURDIR))

COMMIT_ID = $(shell git rev-parse --short HEAD 2> /dev/null)
VERSION   = $(shell git describe --tags --abbrev=0 2> /dev/null | sed 's/v//g')

# Release helpers. `verinc -get <prog>.go` prints the version embedded in the
# program source (e.g. "2.2.1"). settag tags HEAD with `v<version>`; pushtag
# pushes that single tag so a tag-triggered CI build can start.
VERINC      ?= verinc
SRC_VERSION  = $(shell $(VERINC) -get $(PROGRAM).go 2> /dev/null)
TAG          = v$(SRC_VERSION)
GIT_REMOTE  ?= origin

# Host OS, used to decide whether external strip is safe. GNU strip can
# only safely strip binaries for the host format; cross-compiled PE / Mach-O
# binaries must be left alone (Go's -s -w already strips the symbol table).
HOST_OS = $(shell uname -s | tr 'A-Z' 'a-z' | sed -e 's/darwin.*/darwin/' -e 's/linux.*/linux/' -e 's/cygwin.*/windows/' -e 's/mingw.*/windows/')

# Tip: Each command is run on its own line so you can't CD unless you
# connect commands together using operators. See examples:
# A; B    # Run A and then B, regardless of success of A
# A && B  # Run B if and only if A succeeded
# A || B  # Run B if and only if A failed
# A &     # Run A in background.
# Source: https://askubuntu.com/a/539293
#
# Tip: Use $(shell app param) syntax when expanding a shell return value.
#
# Tip: Add `set -e` at the start of a compound recipe to abort on the first
# failing command. Without it, only the exit status of the last command in
# the recipe is checked by make. `cmd || echo fallback` still works because
# the `||` chain returns 0.
#
# Tip: `## <text>` at the end of a target line feeds the help screen below.
#      `##@ <group>` starts a new section. This is the standard
#      "self-documenting Makefile" convention (works with awk on every OS).

# Compiler settings
GO     = go
UPX    = upx --best --force --overlay=strip
STRIP  = strip --strip-all
LDFLAGS = -s -w -buildid=

# Cross-compilation environment, exported to every recipe line.
export CGO_ENABLED = 0
GOARM ?= 6
export GOARM

# Optional ldflags stamping -- uncomment if the program exposes these vars.
# LDFLAGS += -X main.version=$(VERSION) -X main.commit=$(COMMIT_ID)

# Platforms. GOARM=6 covers Raspberry Pi 1, A/B, A+, Zero (32-bit ARM).
PLATFORMS  = linux/amd64 linux/386 darwin/amd64 darwin/arm64 windows/amd64 windows/386 linux/arm64 linux/arm
OUTPUT_DIR = build

.PHONY: help all clean tidy distclean install fmt vet test pdf \
        settag pushtag release $(PLATFORMS)

# Default target: show help. Use `make all` for a full cross-build.
help: ## Show this help screen
	@echo '$(PROGRAM) - build helpers @(#) $$Id:$$'
	@echo "Commit:  $(COMMIT_ID)"
	@echo "Version: $(VERSION)"
	@echo "Source version: $(SRC_VERSION) ($(PROGRAM).go)"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"} \
		/^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
		/^[a-zA-Z0-9_\/-]+:.*##/ {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Platforms: $(PLATFORMS)"

##@ Build

all: $(PLATFORMS) ## Build all platforms into $(OUTPUT_DIR)/ (parallel-safe, use -j)
	@echo "Commit:  $(COMMIT_ID)"
	@echo "Version: $(VERSION)"
	@echo "[Info] Build completed."

# Build one platform. `|`-prerequisite makes OUTPUT_DIR order-only: it is
# created first, but its timestamp is ignored, so parallel runs cannot race.
$(PLATFORMS): | $(OUTPUT_DIR)
	@set -e; \
	os=$(word 1, $(subst /, ,$@)); arch=$(word 2, $(subst /, ,$@)); \
	output_name=$(OUTPUT_DIR)/$(PROGRAM)-$$os-$$arch; \
	if [ "$$os" = "windows" ]; then output_name=$$output_name.exe; fi; \
	echo "Building for $$os/$$arch..."; \
	GOOS=$$os GOARCH=$$arch $(GO) build -o $$output_name -ldflags "$(LDFLAGS)" $(PROGRAM).go; \
	if [ "$$os" = "$(HOST_OS)" ]; then \
	  echo "Stripping binary..."; \
	  $(STRIP) $$output_name || echo "[!!] Skipping strip (failed)"; \
	else \
	  echo "[Info] Skipping strip (cross-compiled; -ldflags already strips)"; \
	fi; \
	echo "Compressing binary with UPX (optional)..."; \
	$(UPX) $$output_name || echo "[!!] Skipping UPX compression (not available/failed)"; \
	echo "Built: $$output_name"

$(OUTPUT_DIR):
	@mkdir -p $(OUTPUT_DIR)

install: all ## Install the native linux/amd64 build to $(HOME)/bin
	@mkdir -p $(HOME)/bin
	cp $(OUTPUT_DIR)/$(PROGRAM)-linux-amd64 $(HOME)/bin/$(PROGRAM)
	@echo "[Info] Installed $(HOME)/bin/$(PROGRAM)"

##@ Release

# settag is idempotent: if v<version> already exists at HEAD it skips; if it
# exists at a different commit it aborts so you bump the version first. This
# makes `make release` re-runnable after a failed push.
settag: ## Create annotated tag v<src version> (skips if already at HEAD)
	@set -e; \
	if ! command -v $(VERINC) >/dev/null 2>&1; then \
	  echo "[!!] $(VERINC) not found in PATH"; exit 1; \
	fi; \
	if [ -z "$(SRC_VERSION)" ]; then \
	  echo "[!!] Could not read version from $(PROGRAM).go via '$(VERINC) -get'"; \
	  exit 1; \
	fi; \
	tag="$(TAG)"; \
	if git rev-parse -q --verify "refs/tags/$$tag" >/dev/null; then \
	  existing=$$(git rev-list -n1 "$$tag"); \
	  head=$$(git rev-parse HEAD); \
	  if [ "$$existing" = "$$head" ]; then \
	    echo "[Info] Tag $$tag already exists at HEAD -- skipping"; \
	  else \
	    echo "[!!] Tag $$tag exists at $$existing, but HEAD is $$head"; \
	    echo "     Bump the version in $(PROGRAM).go and commit before releasing."; \
	    exit 1; \
	  fi; \
	else \
	  if ! git diff --quiet || ! git diff --cached --quiet; then \
	    echo "[!!] Warning: working tree is dirty -- $$tag will point at HEAD"; \
	    echo "     Commit your version bump first if this is unintentional."; \
	  fi; \
	  echo "[Info] Tagging $$tag (source version $(SRC_VERSION))..."; \
	  git tag -a "$$tag" -m "Release $$tag"; \
	  echo "[Info] Created tag $$tag"; \
	fi

# pushtag is idempotent: if the tag is already on the remote it skips, so
# re-running after a network failure is harmless. Warns if the remote is
# HTTPS (the usual cause of an interactive "Username for" prompt).
pushtag: ## Push $(TAG) to $(GIT_REMOTE) (skips if already there)
	@set -e; \
	if [ -z "$(SRC_VERSION)" ]; then \
	  echo "[!!] Could not read version from $(PROGRAM).go via '$(VERINC) -get'"; \
	  exit 1; \
	fi; \
	tag="$(TAG)"; \
	if ! git rev-parse -q --verify "refs/tags/$$tag" >/dev/null; then \
	  echo "[!!] Tag $$tag does not exist -- run 'make settag' first"; \
	  exit 1; \
	fi; \
	remote_url=$$(git remote get-url $(GIT_REMOTE) 2>/dev/null || true); \
	case "$$remote_url" in \
	  https://*) \
	    echo "[Info] Remote $(GIT_REMOTE) uses HTTPS ($$remote_url)."; \
	    echo "       If push prompts for a password, configure a credential"; \
	    echo "       helper or switch to SSH:"; \
	    echo "       git remote set-url $(GIT_REMOTE) git@github.com:USER/REPO.git" ;; \
	esac; \
	if git ls-remote --exit-code --tags $(GIT_REMOTE) "refs/tags/$$tag" >/dev/null 2>&1; then \
	  echo "[Info] Tag $$tag already on $(GIT_REMOTE) -- nothing to push"; \
	  exit 0; \
	fi; \
	echo "[Info] Pushing $$tag to $(GIT_REMOTE)..."; \
	git push $(GIT_REMOTE) "$$tag"; \
	echo "[Info] Pushed $$tag -- tag-triggered CI build should start"

release: settag pushtag ## Tag and push (re-runnable if the push half failed)
	@echo "[Info] Release $(TAG) published."

##@ Housekeeping

clean: ## Remove build artifacts (keeps sources untouched)
	@echo "[!!] Cleaning build artifacts..."
	rm -rf $(OUTPUT_DIR)
	rm -f $(PROGRAM).pdf

tidy: ## Run go mod tidy, go fmt and go vet
	@echo "[Info] Tidying module and formatting sources..."
	$(GO) mod tidy
	$(GO) fmt ./...
	$(GO) vet ./...

distclean: clean ## clean + remove generated module files
	@echo "[!!] Removing generated module files..."
	rm -f go.sum

##@ Quality

fmt: ## Format all Go sources
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

test: ## Run go test
	$(GO) test ./...

pdf: $(PROGRAM).md ## Generate PDF from $(PROGRAM).md
	@echo "[Info] Generating PDF documentation..."
	markdown2pdf -r $(PROGRAM).md || echo "[!!] markdown2pdf not available"

	