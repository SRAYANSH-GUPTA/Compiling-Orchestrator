.PHONY: build run agent tidy migrate

build:
	go build -o bin/dashboard ./cmd/dashboard
	GOOS=linux GOARCH=amd64 go build -o bin/agent-linux-amd64 ./cmd/agent

run:
	go run ./cmd/dashboard

agent:
	go run ./cmd/agent

tidy:
	go mod tidy
