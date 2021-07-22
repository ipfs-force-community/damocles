build-venus-selaer:
	rm -rf ./dist/bin/venus-sealer
	mkdir -p ./dist/bin/
	$(MAKE) -C ./venus-sealer/ build-venus-selaer
	mv ./venus-sealer/venus-sealer ./dist/bin/
