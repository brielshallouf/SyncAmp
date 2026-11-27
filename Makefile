# Requires Go >= the version in go.mod (Go downloads the toolchain automatically).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# -trimpath: no build-machine paths in binaries; CGO off: static, no C attack surface.
GOFLAGS_REL := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: test vuln build dist clean
test:
	go vet ./... && go test -race ./...

build:
	CGO_ENABLED=0 go build $(GOFLAGS_REL) -ldflags "$(LDFLAGS)" -o dist/syncamp ./cmd/syncamp
	CGO_ENABLED=0 go build $(GOFLAGS_REL) -ldflags "$(LDFLAGS)" -o dist/syncamp-tray ./cmd/syncamp-tray

# Scans the *compiled binaries* (code + dependencies + Go stdlib actually shipped).
vuln: dist
	go run golang.org/x/vuln/cmd/govulncheck@latest -mode=binary dist/syncamp-linux-amd64
	go run golang.org/x/vuln/cmd/govulncheck@latest -mode=binary dist/syncamp-windows-amd64.exe

dist: test
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build $(GOFLAGS_REL) -ldflags "$(LDFLAGS)" -o dist/syncamp-linux-amd64 ./cmd/syncamp
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build $(GOFLAGS_REL) -ldflags "$(LDFLAGS)" -o dist/syncamp-linux-arm64 ./cmd/syncamp
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS_REL) -ldflags "$(LDFLAGS)" -o dist/syncamp-windows-amd64.exe ./cmd/syncamp
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(GOFLAGS_REL) -ldflags "$(LDFLAGS)" -o dist/syncamp-windows-arm64.exe ./cmd/syncamp
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS_REL) -ldflags "$(LDFLAGS) -H=windowsgui" -o dist/syncamp-tray-windows-amd64.exe ./cmd/syncamp-tray
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build $(GOFLAGS_REL) -ldflags "$(LDFLAGS)" -o dist/syncamp-tray-linux-amd64 ./cmd/syncamp-tray
	cd dist && sha256sum syncamp-* > SHA256SUMS

clean:
	rm -rf dist
