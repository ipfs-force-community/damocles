unexport GOTAGS
unexport GOFLAGS
BUILD_GIT_FALLBACK := "Unknown (no git or not git repo)"
export GIT_COMMIT?=$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2> /dev/null || git rev-parse --short HEAD 2> /dev/null || echo ${BUILD_GIT_FALLBACK}))

GO?=go

FFI_PATH:=extern/filecoin-ffi/
FFI_DEPS:=.install-filcrypto
FFI_DEPS:=$(addprefix $(FFI_PATH),$(FFI_DEPS))

$(FFI_DEPS): build-dep/.filecoin-install ;

MODULES:=

CLEAN:=
BINS:=
BUILDS:=

build-dep/.filecoin-install: $(FFI_PATH)
	$(MAKE) -C $(FFI_PATH) $(FFI_DEPS:$(FFI_PATH)%=%)
	@touch $@

MODULES+=$(FFI_PATH)
BUILD_DEPS+=build-dep/.filecoin-install
CLEAN+=build-dep/.filecoin-install

$(MODULES): build-dep/.update-modules ;

# dummy file that marks the last time modules were updated
build-dep/.update-modules:
	git submodule update --init --recursive
	touch $@

CLEAN+=build-dep/.update-modules

clean:
	rm -rf $(CLEAN) $(BINS)
	-$(MAKE) -C $(FFI_PATH) clean
.PHONY: clean

ldflags=-X=github.com/ipfs-force-community/damocles/damocles-manager/ver.Commit=git.${GIT_COMMIT}
ifneq ($(strip $(LDFLAGS)),)
	ldflags+=-extldflags=$(LDFLAGS)
endif

GOFLAGS+=-ldflags="$(ldflags)"


build-damocles-manager:
	rm -rf ./damocles-manager
	$(GO) build -tags="$(GOTAGS)" $(GOFLAGS) -o damocles-manager ./cmd/damocles-manager

build_debug: GOFLAGS+=-gcflags="all=-N -l"
build_debug: GOTAGS+=prod
build_debug: build-damocles-manager

.PHONY: damocles-manager
BINS+=damocles-manager
BUILDS+=build-damocles-manager

build-all: GOTAGS+=prod
build-all: $(BUILD_DEPS) $(BUILDS)

build-all-dev: $(MODULES) $(BUILDS)

test-all: GOTAGS+=prod
test-all: $(BUILD_DEPS)
	$(GO) test -tags="$(GOTAGS)" $(GOFLAGS) -v -failfast `go list ./... | grep -v /extern/`

check-all: check-lint check-go-mod

check-lint: $(BUILD_DEPS)
	golangci-lint run -c .golangci.yml

check-go-mod:
	$(GO) mod tidy

dev-env:
	$(GO) install -v github.com/golangci/golangci-lint/cmd/golangci-lint@v1.54.2
