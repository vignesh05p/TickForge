.PHONY: build run simulator test vet fmt tidy

build:
	go build ./cmd/server ./cmd/simulator

run:
	go run ./cmd/server

simulator:
	go run ./cmd/simulator

test:
	go test ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy
