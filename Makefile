.PHONY: all deps build server clean test

all: build

deps:
	go mod download
	go mod tidy

build: server

server:
	go build -o bin/uplink-server ./cmd/uplink-server

server-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/uplink-server-amd64 ./cmd/uplink-server

server-linux-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o bin/uplink-server-arm ./cmd/uplink-server

clean:
	rm -rf bin/

test:
	go test -v ./...

run:
	./bin/uplink-server -config configs/config.example.yml
