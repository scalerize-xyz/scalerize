STARTING-IP-ADDR := 172.20.0.2
NODES := 4
SCALERIZED_BINARY_PATH := /go/src/github.com/aerius-labs/scalerize/build/scalerized
BUILDDIR ?= $(CURDIR)/build

include scripts/execution-client.mk

BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
COMMIT := $(shell git log -1 --format='%H')

ifeq (,$(VERSION))
  VERSION := $(shell git describe --exact-match 2>/dev/null)
  ifeq (,$(VERSION))
    VERSION := $(BRANCH)-$(COMMIT)
  endif
endif

ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=scalerize \
	-X github.com/cosmos/cosmos-sdk/version.AppName=scalerized \
	-X github.com/cosmos/cosmos-sdk/version.Version=$(VERSION) \
	-X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT)

BUILD_FLAGS := -ldflags '$(ldflags)'

###########
# Install #
###########

.PHONY: build install

install:
	@echo "--> ensure dependencies have not been modified"
	@go mod verify
	@echo "--> installing scalerized"
	@go install $(BUILD_FLAGS) -mod=readonly ./cmd/scalerized

###########
# Build #
###########

build: BUILD_ARGS=-o $(BUILDDIR)/
build-linux:
	GOOS=linux GOARCH=amd64 LEDGER_ENABLED=false $(MAKE) build

build: go.sum $(BUILDDIR)/
	go build $(BUILD_FLAGS) $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/

###########
# Misc #
###########

init:
	./scripts/init.sh

localtestnet-example-config: 
	$(SCALERIZED_BINARY_PATH) testnet init-files --output-dir example-testnet --v $(NODES) --starting-ip-address $(STARTING-IP-ADDR) --keyring-backend test
