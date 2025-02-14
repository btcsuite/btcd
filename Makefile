PKG := github.com/btcsuite/btcd

LINT_PKG := github.com/golangci/golangci-lint/cmd/golangci-lint
GOIMPORTS_PKG := golang.org/x/tools/cmd/goimports

GO_BIN := $(shell go env GOPATH)/bin
LINT_BIN := $(GO_BIN)/golangci-lint
GOIMPORTS_BIN := $(GO_BIN)/goimports

GOBUILD := go build -v
GOINSTALL := go install -v
DEV_TAGS := rpctest
GOTEST_DEV = go test -v -tags=$(DEV_TAGS)
GOTEST := go test -v
GOMODTIDY := go mod tidy

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

#? default: Run `make build`
default: build

#? all: Run `make build` and `make check`
all: build check

# ============
# DEPENDENCIES
# ============

$(LINT_BIN):
	@$(call print, "Fetching linter")
	$(GOINSTALL) $(LINT_PKG)@latest

#? goimports: Install goimports
goimports:
	@$(call print, "Installing goimports.")
	$(GOINSTALL) $(GOIMPORTS_PKG)@latest

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

#? release-install: Install btcd and btcctl release binaries, place them in $GOPATH/bin
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
	cd btcec && $(GOTEST_DEV) ./... -test.timeout=20m
	cd btcutil && $(GOTEST_DEV) ./... -test.timeout=20m
	cd btcutil/psbt && $(GOTEST_DEV) ./... -test.timeout=20m

#? unit-cover: Run unit coverage tests
unit-cover:
	@$(call print, "Running unit coverage tests.")
	$(GOTEST) -coverprofile=coverage.txt ./...

	# We need to remove the /v2 pathing from the module to have it work
	# nicely with the CI tool we use to render live code coverage.
	cd btcec && $(GOTEST) -coverprofile=coverage.txt ./... && sed -i.bak 's/v2\///g' coverage.txt
	cd btcutil && $(GOTEST) -coverprofile=coverage.txt ./...
	cd btcutil/psbt && $(GOTEST) -coverprofile=coverage.txt ./...

#? unit-race: Run unit race tests
unit-race:
	@$(call print, "Running unit race tests.")
	env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd btcec && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd btcutil && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd btcutil/psbt && env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...

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
	rm -f coverage.txt btcec/coverage.txt btcutil/coverage.txt btcutil/psbt/coverage.txt
	
#? tidy-module: Run 'go mod tidy' for all modules
tidy-module:
	@$(call print, "Running 'go mod tidy' for all modules")
	$(GOMODTIDY)
	cd btcec && $(GOMODTIDY)
	cd btcutil && $(GOMODTIDY)
	cd btcutil/psbt && $(GOMODTIDY)

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
	tidy-module

#? help: Get more info on make commands
help: Makefile
	@echo " Choose a command run in btcd:"
	@sed -n 's/^#?//p' $< | column -t -s ':' |  sort | sed -e 's/^/ /'

.PHONY: help
