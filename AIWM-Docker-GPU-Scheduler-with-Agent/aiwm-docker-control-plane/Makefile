.PHONY: run agent sim benchmark test race vet build check clean

run:
	go run ./cmd/aiwm-server

sim:
	go run ./cmd/aiwm-agent-sim

agent:
	go run ./cmd/aiwm-agent

benchmark:
	go run ./cmd/aiwm-scheduler-bench

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

build:
	mkdir -p bin
	go build -o bin/aiwm-server ./cmd/aiwm-server
	go build -o bin/aiwm-agent ./cmd/aiwm-agent
	go build -o bin/aiwm-agent-sim ./cmd/aiwm-agent-sim
	go build -o bin/aiwm-scheduler-bench ./cmd/aiwm-scheduler-bench

check: test race vet build

clean:
	go clean
	rm -rf bin coverage.out
