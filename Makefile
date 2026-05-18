.PHONY: build test run-server run-cli tidy

build:
	go build -o bin/server ./cmd/server/
	go build -o bin/krk ./cmd/krk/

test:
	go test ./... -count=1

run-server:
	./bin/server

tidy:
	go mod tidy
