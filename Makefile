.SILENT :
.PHONY : dockerize clean fmt lint test

TAG := $(shell git describe --abbrev=0 --tags 2>/dev/null || echo dev)
LDFLAGS := -X main.buildVersion=$(TAG)
GO111MODULE := on

all: lint test dockerize

deps:
	go mod tidy

lint:
	golangci-lint run ./...

test:
	go test -v -race ./...

dockerize:
	echo "Building dockerize"
	go install -ldflags "$(LDFLAGS)"

clean:
	rm -rf dist
	rm -f dockerize-*.tar.gz
