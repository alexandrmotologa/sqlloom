.PHONY: all build build-ui build-go test clean run docker-build

all: build

build-ui:
	cd web && npm ci && npm run build

build-go:
	go build -ldflags="-s -w" -o sqlloom cmd/sqlloom/main.go

build: build-ui build-go

test:
	go test -v ./...

clean:
	rm -rf sqlloom sqlloom.exe internal/ui/dist web/node_modules

run: build
	./sqlloom -listen :5433 -upstream 127.0.0.1:5432 -dashboard :8080

docker-build:
	docker build -t alexandrmotologa/sqlloom:latest .
