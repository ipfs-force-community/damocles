build-venus-selaer:
	rm -rf ./dist/bin/venus-sealer
	mkdir -p ./dist/bin/
	cd venus-sealer && go build -o ../dist/bin/venus-sealer ./cmd/venus-sealer
