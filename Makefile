PKG := github.com/btcsuite/btcd

LINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOIMPORTS_PKG := golang.org/x/tools/cmd/goimports

GO_BIN := ${shell go env GOBIN}

# If GOBIN is not set, default to GOPATH/bin.
ifeq ($(GO_BIN),)
GO_BIN := $(shell go env GOPATH)/bin
endif

LINT_BIN := $(GO_BIN)/golangci-lint
GOIMPORTS_BIN := $(GO_BIN)/goimports

LINT_COMMIT := v2.1.6
GOIMPORTS_COMMIT := a24facf9e5586c95743d2f4ad15d148c7a8cf00b

GOBUILD := go build -v
GOINSTALL := go install -v 
DEV_TAGS := rpctest
GOTEST_DEV = go test -v -tags=$(DEV_TAGS)
GOTEST := go test -v
COVER_FLAGS = -coverprofile=coverage.txt -covermode=atomic -coverpkg=$(PKG)/...
MODULES := address btcec btcutil chaincfg chainhash descriptors psbt txscript v2transport wire

# Linting uses a lot of memory, so keep it under control by limiting the number
# of workers if requested.
ifneq ($(workers),)
LINT_WORKERS = --concurrency=$(workers)
endif
LINT_TIMEOUT := 5m

LINT = $(LINT_BIN) run -v $(LINT_WORKERS) --timeout=$(LINT_TIMEOUT)

GREEN := "\\033[0;32m"
NC := "\\033[0m"
define print
	echo $(GREEN)$1$(NC)
endef

# Time budget per fuzz target for go-fuzz. Override with `fuzztime=10m` for a
# long (nightly-style) run; the default is a quick local smoke.
fuzztime ?= 15s

#? default: Run `make build`
default: build

#? all: Run `make build` and `make check`
all: build check

# ============
# DEPENDENCIES
# ============

$(LINT_BIN):
	@$(call print, "Fetching linter")
	$(GOINSTALL) $(LINT_PKG)@$(LINT_COMMIT)

#? goimports: Install goimports
goimports:
	@$(call print, "Installing goimports.")
	$(GOINSTALL) $(GOIMPORTS_PKG)@$(GOIMPORTS_COMMIT)

# ============
# INSTALLATION
# ============

#? build: Build all binaries, place them in project directory
build:
	@$(call print, "Building all binaries")
	$(GOBUILD) $(PKG)
	$(GOBUILD) $(PKG)/cmd/btcctl
	$(GOBUILD) $(PKG)/cmd/gencerts
	$(GOBUILD) $(PKG)/cmd/findcheckpoint
	$(GOBUILD) $(PKG)/cmd/addblock

#? install: Install all binaries, place them in $GOPATH/bin
install:
	@$(call print, "Installing all binaries")
	$(GOINSTALL) $(PKG)
	$(GOINSTALL) $(PKG)/cmd/btcctl
	$(GOINSTALL) $(PKG)/cmd/gencerts
	$(GOINSTALL) $(PKG)/cmd/findcheckpoint
	$(GOINSTALL) $(PKG)/cmd/addblock

#? release-install: Install btcd and btcctl release binaries, place them in $GOBIN
release-install:
	@$(call print, "Installing btcd and btcctl release binaries")
	env CGO_ENABLED=0 $(GOINSTALL) -trimpath -ldflags="-s -w -buildid=" $(PKG)
	env CGO_ENABLED=0 $(GOINSTALL) -trimpath -ldflags="-s -w -buildid=" $(PKG)/cmd/btcctl

# =======
# TESTING
# =======

#? check: Run `make unit`
check: unit

#? unit: Run unit tests
unit:
	@$(call print, "Running unit tests.")
	$(GOTEST_DEV) ./... -test.timeout=20m
	for module in $(MODULES); do \
		( cd $$module; echo $$module; \
		  $(GOTEST_DEV) ./... -test.timeout=20m \
		); \
	done

#? unit-cover: Run unit coverage tests
unit-cover:
	@$(call print, "Running unit coverage tests.")
	$(GOTEST) $(COVER_FLAGS) ./...

	# We need to remove the /v2 pathing from the module to have it work
	# nicely with the CI tool we use to render live code coverage.
	for module in $(MODULES); do \
		( cd $$module; echo $$module; \
		  $(GOTEST) $(COVER_FLAGS) ./... && sed -i.bak 's/v2\///g' coverage.txt \
		); \
	done

#? unit-race: Run unit race tests
unit-race:
	@$(call print, "Running unit race tests.")
	env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...

	for module in $(MODULES); do \
		( cd $$module; echo $$module; \
		  env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./... \
	    ); \
	done

#? go-fuzz: Run every Fuzz* target, coverage-guided, for `fuzztime` (default 15s) each. Seed corpora always run as part of go-unit; this target is the mutation engine on top.
go-fuzz:
	@set -e; \
	for module in $(MODULES); do \
		( cd $$module; \
		for pkg in $$(go list ./...); do \
			for target in $$(go test -list='^Fuzz' $$pkg \
				| grep '^Fuzz' || true); do \
				echo "=== go-fuzz: $$target ($$pkg)"; \
				go test -run='^$$' -fuzz="^$$target\$$" \
					-fuzztime=$(fuzztime) $$pkg; \
			done; \
		done ); \
	done

# =========
# UTILITIES
# =========

#? fmt: Fix imports and formatting source
fmt: goimports
	@$(call print, "Fixing imports.")
	$(GOIMPORTS_BIN) -w .
	@$(call print, "Formatting source.")
	gofmt -l -w -s .

#? lint: Lint source
lint: $(LINT_BIN)
	@$(call print, "Linting source.")
	$(LINT)

#? clean: Clean source
clean:
	@$(call print, "Cleaning source.$(NC)")
	find . -name coverage.txt | xargs $(RM)
	find . -name coverage.txt.bak | xargs $(RM)

#? tidy-module: Run 'go mod tidy' for all modules
tidy-module:
	echo "Running 'go mod tidy' for all modules"
	scripts/tidy_modules.sh

.PHONY: all \
	default \
	build \
	check \
	unit \
	unit-cover \
	unit-race \
	fmt \
	lint \
	clean \
	go-fuzz \
	tidy-module

#? help: Get more info on make commands
help: Makefile
	@echo " Choose a command run in btcd:"
	@sed -n 's/^#?//p' $< | column -t -s ':' |  sort | sed -e 's/^/ /'

.PHONY: help
