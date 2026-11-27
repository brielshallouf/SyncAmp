VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: test build dist
test:
	go vet ./... && go test -race ./...
build:
	go build -ldflags "$(LDFLAGS)" -o dist/syncamp ./cmd/syncamp
dist: test
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS) -H=windowsgui" -o dist/syncamp-windows-amd64.exe ./cmd/syncamp
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/syncamp-linux-amd64 ./cmd/syncamp
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/syncamp-linux-arm64 ./cmd/syncamp
