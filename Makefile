# Variables
API_BINARY=api
WORKER_BINARY=worker
UI_DIR=ui
BUILD_DIR=build
DOCKER_COMPOSE=docker-compose.yml

# Go variables
GO=go
GO_BUILD=$(GO) build
GO_TEST=$(GO) test
GO_MOD=$(GO) mod
GOFMT=$(GO) fmt

.PHONY: all build build-api build-worker test clean fmt docker-build docker-up docker-down docker-logs ui

all: build

build: build-api build-worker

build-api:
	$(GO_BUILD) -o $(BUILD_DIR)/$(API_BINARY) ./cmd/api

build-worker:
	$(GO_BUILD) -o $(BUILD_DIR)/$(WORKER_BINARY) ./cmd/worker

docker-build:
	docker-compose -f $(DOCKER_COMPOSE) build

docker-up:
	docker-compose -f $(DOCKER_COMPOSE) up -d

docker-down:
	docker-compose -f $(DOCKER_COMPOSE) down

deps:
	$(GO_MOD) download
	$(GO_MOD) tidy