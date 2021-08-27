unexport GOTAGS

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


build-venus-sector-manager:
	rm -rf ./venus-sector-manager
	go build -tags "$(GOTAGS)" -o venus-sector-manager ./cmd/venus-sector-manager

.PHONY: venus-sector-manager
BINS+=venus-sector-manager
BUILDS+=build-venus-sector-manager

build-all: GOTAGS+=prod
build-all: $(BUILD_DEPS) $(BUILDS)

build-all-dev: $(MODULES) $(BUILDS)
