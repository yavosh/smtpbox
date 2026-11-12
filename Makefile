DOCKER_COMPOSE_YAML := docker-compose.yaml
GOLANGCI_LINT_VERSION := 2.15.0
GOLANGCI_LINT ?= golangci-lint

help:
	@echo "Please use 'make <target>' where <target> is one of the following:"
	@echo "  up			to serve the app."
	@echo "  down		to stop serving the app."
	@echo "  build		to build the binaries."
	@echo "  clean		to clean the binaries."
	@echo "  fmt		to perform formatting."
	@echo "  fmt-check	to check formatting."
	@echo "  lint		to perform linting."
	@echo "  test		to run the tests."
	@echo "  cover		to run the tests with code coverage."

up:
	docker-compose -f $(DOCKER_COMPOSE_YAML) up --build api

down:
	docker-compose -f $(DOCKER_COMPOSE_YAML) down

build:
	go build -o bin/smtpbox ./cmd/smtpbox
	go build -o bin/sendmail ./cmd/sendmail

clean:
	rm -f ./bin/*

check-lint-tool:
	@test "$$($(GOLANGCI_LINT) version --short)" = "$(GOLANGCI_LINT_VERSION)" || { echo "Install golangci-lint v$(GOLANGCI_LINT_VERSION)."; exit 1; }

fmt: check-lint-tool
	$(GOLANGCI_LINT) fmt ./...

fmt-check: check-lint-tool
	$(GOLANGCI_LINT) fmt --diff ./...

lint: check-lint-tool
	$(GOLANGCI_LINT) run ./...

test:
	go test -race ./...

cover:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...

.PHONY: help up down build clean check-lint-tool fmt fmt-check lint test cover
