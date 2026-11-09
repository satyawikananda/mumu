GO ?= go

.PHONY: dev build test vet fmt fmt-check lint ci

dev:
	$(GO) tool mygo dev

build:
	$(GO) tool mygo build

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...
	$(GO) tool mygo vet .

fmt:
	$(GO) fmt ./cmd/... ./internal/...

fmt-check:
	@out="$$(gofmt -l ./cmd ./internal)"; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

lint:
	golangci-lint run ./cmd/... ./internal/...

ci: fmt-check vet test
