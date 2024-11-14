.PHONY: run build

run:
	go run ./cmd/processor/main.go

build:
	go build -o bin/ton-block-processor ./cmd/processor