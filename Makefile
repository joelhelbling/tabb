BINARY := tabb
PKG    := ./cmd/tabb
PREFIX ?= /usr/local

.PHONY: build install uninstall test clean release-snapshot

build:
	go build -ldflags "-X main.Version=dev" -o $(BINARY) $(PKG)

install: build
	install -d $(PREFIX)/bin
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)

test:
	go test ./...

clean:
	rm -f $(BINARY)

release-snapshot:
	goreleaser release --snapshot --clean
