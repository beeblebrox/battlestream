OPENDECK_PLUGINS := $(HOME)/.var/app/me.amankhanna.opendeck/config/opendeck/plugins

.PHONY: build build-plugin build-all install-plugin test vet

build:
	go build ./cmd/battlestream

build-plugin:
	bash scripts/build-plugin.sh

build-all: build build-plugin

install-plugin: build-plugin
	rm -rf "$(OPENDECK_PLUGINS)/com.battlestream.streamdeck.sdPlugin"
	cp -r streamdeck-plugin/dist/com.battlestream.streamdeck.sdPlugin "$(OPENDECK_PLUGINS)/"
	@echo "Installed. Restart OpenDeck to pick up changes."

test:
	go test -count=1 ./...

vet:
	go vet ./...
