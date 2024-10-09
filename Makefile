STARTING-IP-ADDR := 172.20.0.2 
NODES := 4
SCALERIZED_CONTAINER_DIR := /go/src/github.com/aerius-labs/scalerize/build/scalerized
BUILDDIR ?= $(CURDIR)/build

include scripts/execution-client.mk


BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
COMMIT := $(shell git log -1 --format='%H')

# don't override user values
ifeq (,$(VERSION))
  VERSION := $(shell git describe --exact-match 2>/dev/null)
  # if VERSION is empty, then populate it with branch's name and raw commit hash
  ifeq (,$(VERSION))
    VERSION := $(BRANCH)-$(COMMIT)
  endif
endif

# Update the ldflags with the app, client & server names
ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=scalerize \
	-X github.com/cosmos/cosmos-sdk/version.AppName=scalerized \
	-X github.com/cosmos/cosmos-sdk/version.Version=$(VERSION) \
	-X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT)

BUILD_FLAGS := -ldflags '$(ldflags)'

###########
# Install #
###########

.PHONY: build install

BUILD_TARGETS := build

install:
	@echo "--> ensure dependencies have not been modified"
	@go mod verify
	@echo "--> building scalerized"
	@go build install $(BUILD_FLAGS) -mod=readonly ./cmd/scalerized 


BUILD_TARGETS := build

build: BUILD_ARGS=-o $(BUILDDIR)/
build-linux:
	GOOS=linux GOARCH=amd64 LEDGER_ENABLED=false $(MAKE) build

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	go $@ $(BUILD_FLAGS) $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/
	
init:
	./scripts/init.sh

localtestnet-example-config: 
	$(SCALERIZED_CONTAINER_DIR) testnet init-files --output-dir example-testnet --v $(NODES) --starting-ip-address $(STARTING-IP-ADDR) --keyring-backend test

