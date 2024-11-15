.PHONY: run build

run:
	go run ./cmd/processor/main.go

build:
	go build -o bin/toncenter-block-cacher ./cmd/processor