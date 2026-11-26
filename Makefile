GO ?= go
VERSION ?= $(shell sed -n 's/.*"version": "\([^"]*\)".*/\1/p' mygo.json)
APP ?= build/darwin-arm64/Mumu.app
DMG ?= build/darwin-arm64/mumu-$(VERSION)-arm64.dmg

.PHONY: dev build test vet fmt fmt-check lint ci runtime prepare-runtime \
	package verify-app clean-runtime qa-sample

dev:
	$(GO) tool mygo dev

build:
	$(GO) tool mygo build

test:
	$(GO) test -race ./...
	(cd third_party/go-mpv && $(GO) test ./...)

vet:
	$(GO) vet ./...
	$(GO) tool mygo vet .
	(cd third_party/go-mpv && $(GO) vet ./...)

fmt:
	$(GO) fmt ./cmd/... ./internal/...
	gofmt -w third_party/go-mpv

fmt-check:
	@out="$$(gofmt -l ./cmd ./internal third_party/go-mpv)"; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

lint:
	golangci-lint run ./cmd/... ./internal/...

ci: fmt-check vet test

# Build the LGPL runtime once, then stage it where mygo build copies it.
runtime:
	./packaging/macos/build-lgpl-runtime.sh
	./packaging/macos/prepare-runtime.sh

notices:
	cp THIRD_PARTY_NOTICES.md resources/THIRD_PARTY_NOTICES.md

prepare-runtime:
	./packaging/macos/prepare-runtime.sh

package: runtime
	$(GO) tool mygo build
	./packaging/macos/verify-app.sh $(APP) $(DMG)

verify-app:
	./packaging/macos/verify-app.sh $(APP) $(DMG)

clean-runtime:
	rm -rf build/runtime resources/darwin-arm64/libmpv

qa-sample:
	@test -n "$(PID)" || { echo "usage: make qa-sample PID=<pid> SECONDS=3600 DIR=build/qa"; exit 1; }
	./packaging/qa/sample.sh $(PID) $(or $(SECONDS),3600) $(or $(DIR),build/qa)
