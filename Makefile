BINARY := freehire
DIST := dist
VERSION := $(shell git describe --tags --dirty --always)
LDFLAGS := -X github.com/strelov1/freehire-cli/internal/cli.Version=$(VERSION)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: build
build:
	@mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "building $(DIST)/$(BINARY)_$${os}_$${arch}"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o $(DIST)/$(BINARY)_$${os}_$${arch} ./cmd/$(BINARY); \
	done
