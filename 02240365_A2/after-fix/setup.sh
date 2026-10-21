#!/usr/bin/env bash
# One-time setup: generate gRPC code from proto/ and fetch Go dependencies.
# Requires: go, protoc (brew install protobuf), and the two Go protoc plugins:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$PATH:$(go env GOPATH)/bin"

protoc \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  proto/*.proto

go get google.golang.org/grpc google.golang.org/protobuf \
       github.com/aws/aws-sdk-go-v2/config github.com/aws/aws-sdk-go-v2/service/lambda
go mod tidy
go build ./...
echo "Setup complete."
