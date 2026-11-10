.DEFAULT_GOAL := build

TOOLS := $(CURDIR)/.tools/bin
export PATH := $(TOOLS):$(PATH)

.PHONY: tools generate generate-config generate-handlers proto lint test build

tools:
	GOBIN="$(TOOLS)" go install github.com/bufbuild/buf/cmd/buf@v1.50.0
	GOBIN="$(TOOLS)" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
	GOBIN="$(TOOLS)" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	GOBIN="$(TOOLS)" go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.26.3

generate: proto generate-handlers generate-config

generate-config:
	go run ./tools/config

generate-handlers:
	go run ./tools/handlergen

proto:
	buf generate

lint:
	buf lint
	go vet ./...

test:
	go test ./...

build:
	go build -o bin/SoupWebApplication ./cmd/SoupWebApplication

