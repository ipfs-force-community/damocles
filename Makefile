build-all:
	rm -rf ./dist/bin/venus-sealer
	mkdir -p ./dist/bin/
	$(MAKE) -C ./venus-sealer/ build-all
	mv ./venus-sealer/venus-sealer ./dist/bin/

claen:
	$(MAKE) -C ./venus-sealer/ clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean
