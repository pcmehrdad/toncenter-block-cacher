.PHONY: run build clean

run:
	go run ./cmd/processor/main.go

build:
	go build -o bin/toncenter-block-cacher ./cmd/processor

clean:
	rm -f bin/toncenter-block-cacher