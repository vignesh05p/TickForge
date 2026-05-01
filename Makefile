.PHONY: run simulator test vet fmt tidy

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
