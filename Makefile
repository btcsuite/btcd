PKG := github.com/btcsuite/btcd

LINT_PKG := github.com/golangci/golangci-lint/cmd/golangci-lint
GOIMPORTS_PKG := golang.org/x/tools/cmd/goimports

GO_BIN := ${GOPATH}/bin
LINT_BIN := $(GO_BIN)/golangci-lint

LINT_COMMIT := v1.18.0

DEPGET := cd /tmp && go install -v
GOBUILD := go build -v
GOINSTALL := go install -v 
DEV_TAGS := rpctest
GOTEST_DEV = go test -v -tags=$(DEV_TAGS)
GOTEST := go test -v
COVER_FLAGS = -coverprofile=coverage.txt -covermode=atomic -coverpkg=$(PKG)/...

GOFILES_NOVENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

RM := rm -f
CP := cp
MAKE := make
XARGS := xargs -L 1

# Linting uses a lot of memory, so keep it under control by limiting the number
# of workers if requested.
ifneq ($(workers),)
LINT_WORKERS = --concurrency=$(workers)
endif

LINT = $(LINT_BIN) run -v $(LINT_WORKERS)

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
	$(DEPGET) $(LINT_PKG)@$(LINT_COMMIT)

#? goimports: Install goimports
goimports:
	@$(call print, "Installing goimports.")
	$(DEPGET) $(GOIMPORTS_PKG)

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
	cd btcec; $(GOTEST_DEV) ./... -test.timeout=20m
	cd btcutil; $(GOTEST_DEV) ./... -test.timeout=20m
	cd btcutil/psbt; $(GOTEST_DEV) ./... -test.timeout=20m

#? unit-cover: Run unit coverage tests
unit-cover:
	@$(call print, "Running unit coverage tests.")
	$(GOTEST) $(COVER_FLAGS) ./...
	# We need to remove the /v2 pathing from the module to have it work
	# nicely with the CI tool we use to render live code coverage.	
	cd btcec; $(GOTEST) $(COVER_FLAGS) ./...; \
		sed -i.bak 's/v2\///g' coverage.txt

	cd btcutil; $(GOTEST) $(COVER_FLAGS) ./...

	cd btcutil/psbt; $(GOTEST) $(COVER_FLAGS) ./...

#? unit-race: Run unit race tests
unit-race:
	@$(call print, "Running unit race tests.")
	env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd btcec; env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd btcutil; env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...
	cd btcutil/psbt; env CGO_ENABLED=1 GORACE="history_size=7 halt_on_errors=1" $(GOTEST) -race -test.timeout=20m ./...

# =========
# UTILITIES
# =========

#? fmt: Fix imports and formatting source
fmt: goimports
	@$(call print, "Fixing imports.")
	goimports -w $(GOFILES_NOVENDOR)
	@$(call print, "Formatting source.")
	gofmt -l -w -s $(GOFILES_NOVENDOR)

#? lint: Lint source
lint: $(LINT_BIN)
	@$(call print, "Linting source.")
	$(LINT)

#? clean: Clean source
clean:
	@$(call print, "Cleaning source.$(NC)")
	$(RM) coverage.txt btcec/coverage.txt btcutil/coverage.txt btcutil/psbt/coverage.txt
	
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
	clean

#? help: Get more info on make commands
help: Makefile
	@echo " Choose a command run in btcd:"
	@sed -n 's/^#?//p' $< | column -t -s ':' |  sort | sed -e 's/^/ /'

.PHONY: help
