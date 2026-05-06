OPENDECK_BASE    := $(HOME)/.var/app/me.amankhanna.opendeck/config/opendeck
OPENDECK_PLUGINS := $(OPENDECK_BASE)/plugins
OPENDECK_PROFILES := $(OPENDECK_BASE)/profiles

.PHONY: build build-plugin build-all install-plugin gen-profiles test vet

build:
	go build ./cmd/battlestream

build-plugin:
	bash scripts/build-plugin.sh

build-all: build build-plugin

gen-profiles:
	cd streamdeck-plugin && node scripts/gen-profiles.mjs

install-plugin: build-plugin
	rm -rf "$(OPENDECK_PLUGINS)/com.battlestream.streamdeck.sdPlugin"
	cp -r streamdeck-plugin/dist/com.battlestream.streamdeck.sdPlugin "$(OPENDECK_PLUGINS)/"
	@for dir in "$(OPENDECK_PROFILES)"/sd-*/; do \
		[ -d "$$dir" ] || continue; \
		cp streamdeck-plugin/profiles/Battlestream*.json "$$dir"; \
		echo "Installed profiles to $$dir"; \
	done
	@echo "Done. Restart OpenDeck to pick up changes."

test:
	go test -count=1 ./...

vet:
	go vet ./...
