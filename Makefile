.PHONY: build test test-short clean

BINARY := cvalent
VERSION := 0.1.0-dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/cvalent

test:
	go test ./... -v -count=1

test-short:
	go test ./... -v -count=1 -short

clean:
	rm -f $(BINARY)
	go clean -testcache
