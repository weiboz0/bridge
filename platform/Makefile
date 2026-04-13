.PHONY: build run dev test test-integration test-contract clean

build:
	go build -o bin/api ./cmd/api

run: build
	./bin/api

dev:
	$(shell go env GOPATH)/bin/air

test:
	go test ./... -count=1 -short

test-integration:
	go test ./... -count=1 -v

test-contract:
	go test ./tests/contract/ -v -count=1

clean:
	rm -rf bin/ tmp/
