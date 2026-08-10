# tree-trunk — build & release helpers
# docs/design/05-implementation-plan.md M5

VERSION ?= dev
GO      ?= go
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test race vet fmt lint cross install bench release-check

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o tree-trunk ./cmd/tree-trunk

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')
	@test -z "$$(gofmt -l .)" || (gofmt -l .; echo "gofmt needed"; exit 1)

# cross-compile matrix (CGO off → static binaries)
CROSS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64
cross:
	@for target in $(CROSS); do \
		GOOS=$${target%/*} GOARCH=$${target#*/} CGO_ENABLED=0 \
			$(GO) build -ldflags "$(LDFLAGS)" \
			-o dist/tree-trunk-$${target%/*}-$${target#*/} ./cmd/tree-trunk || exit 1; \
		echo "built $$target"; \
	done

install:
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/tree-trunk

bench:
	$(GO) test -bench . -benchmem ./internal/discover/

release-check:
	goreleaser check
