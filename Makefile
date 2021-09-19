DOCKER_COMPOSE_YAML := docker-compose.yaml

help:
	@echo "Please use 'make <target>' where <target> is one of the following:"
	@echo "  up			to serve the app."
	@echo "  down		to stop serving the app."
	@echo "  build		to build the binaries."
	@echo "  clean		to clean the binaries."
	@echo "  fmt		to perform formatting."
	@echo "  lint		to perform linting."
	@echo "  test		to run the tests."
	@echo "  cover		to run the tests with code coverage."

up:
	docker-compose -f $(DOCKER_COMPOSE_YAML) up --build

down:
	docker-compose -f $(DOCKER_COMPOSE_YAML) down

build:
	go build -o bin/smtpbox cmd/smtpbox/main.go

clean:
	rm -f ./bin/*

fmt:
	go fmt ./...

lint:
	go vet ./...
	staticcheck -checks="all" `go list ./... | grep -v proto`

test:
	go test `go list ./...` -race

cover:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...

proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative ./proto/*.proto

.PHONY: clean