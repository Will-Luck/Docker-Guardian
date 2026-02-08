.PHONY: build test lint integration-test acceptance-test clean all

BINARY := guardian
IMAGE := docker-guardian

all: lint test build

build:
	CGO_ENABLED=0 go build -o bin/$(BINARY) ./cmd/guardian

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY)-linux-amd64 ./cmd/guardian
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY)-linux-arm64 ./cmd/guardian

test:
	go test -race ./...

lint:
	golangci-lint run

integration-test:
	go test -tags=integration -race ./...

docker-build:
	docker build -t $(IMAGE) .

acceptance-test: docker-build
	GUARDIAN_IMAGE=$(IMAGE) bash tests/test-all.sh

clean:
	rm -rf bin/
