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
	docker-compose -f $(DOCKER_COMPOSE_YAML) up --build api

down:
	docker-compose -f $(DOCKER_COMPOSE_YAML) down

build:
	go build -o bin/smtpbox cmd/smtpbox/main.go
	go build -o bin/sendmail cmd/sendmail/main.go

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

.PHONY: clean