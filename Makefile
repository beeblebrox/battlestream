.PHONY: build build-plugin build-all test vet

build:
	go build ./cmd/battlestream

build-plugin:
	bash scripts/build-plugin.sh

build-all: build build-plugin

test:
	go test -count=1 ./...

vet:
	go vet ./...
