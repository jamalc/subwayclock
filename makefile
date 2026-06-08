.PHONY: clock conway stress gravity fetch serve simulate clean

# TODO: https://tinygo.org/docs/guides/optimizing-binaries/
clock conway stress gravity:
	tinygo flash -monitor -target=matrixportal-m4 -stack-size=8KB ./cmd/$@

# Build the GTFS fetcher binary for deployment
fetch:
	GOOS=linux GOARCH=amd64 go build -o bin/ ./cmd/fetch

# Build the service binary for deployment
serve:
	GOOS=linux GOARCH=amd64 go build -o bin/ ./cmd/serve

# Build the desktop simulator
simulate:
	go build -o bin/ ./cmd/simulate

clean:
	rm -rf ~/Library/Caches/tinygo
	rm -rf ./bin

echo:
	echo $(VALUE)
