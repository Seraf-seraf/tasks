APP_NAME := taskservice
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP_NAME)
MIGRATIONS_DIR := internal/migrations
export GOCACHE ?= /tmp/go-build-cache
export GOMODCACHE ?= /tmp/go-mod-cache

.PHONY: help generate fmt fmt-check vet test test-integration cover cover-critical build run clean db-up db-down db-status migrate compose-up compose-down compose-logs ci

help:
	@echo "Доступные команды:"
	@echo "  make generate         - сгенерировать OpenAPI модели и chi handlers"
	@echo "  make fmt              - отформатировать Go-код"
	@echo "  make fmt-check        - проверить форматирование Go-кода"
	@echo "  make vet              - запустить go vet"
	@echo "  make test             - запустить unit-тесты"
	@echo "  make test-integration - запустить интеграционные тесты"
	@echo "  make cover            - посчитать покрытие тестами"
	@echo "  make cover-critical   - проверить покрытие internal/service >= 85%"
	@echo "  make build            - собрать бинарник"
	@echo "  make run              - запустить сервис локально"
	@echo "  make clean            - удалить локальные build-артефакты"
	@echo "  make db-up            - применить MySQL migrations через goose"
	@echo "  make db-down          - откатить последнюю MySQL migration через goose"
	@echo "  make db-status        - показать статус MySQL migrations через goose"
	@echo "  make migrate DB_DSN=... - применить MySQL migrations"
	@echo "  make compose-up       - поднять docker-compose stack"
	@echo "  make compose-down     - остановить docker-compose stack"
	@echo "  make compose-logs     - показать логи приложения"
	@echo "  make ci               - полный прогон: generate + fmt-check + vet + test + build"

generate:
	go generate ./internal/httpapi

fmt:
	gofmt -w cmd internal

fmt-check:
	@test -z "$$(gofmt -l cmd internal)"

vet:
	go vet ./...

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

cover-critical:
	go test ./internal/service -coverprofile=coverage-critical.out
	@coverage="$$(go tool cover -func=coverage-critical.out | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}')"; \
	awk -v coverage="$$coverage" 'BEGIN { if (coverage < 85) { printf("internal/service coverage %.1f%% is below 85%%\n", coverage); exit 1 } printf("internal/service coverage %.1f%%\n", coverage) }'

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd

run:
	go run ./cmd -config config/config.yaml

clean:
	rm -rf $(BIN_DIR) coverage.out coverage-critical.out

db-up:
	goose -dir $(MIGRATIONS_DIR) mysql "$(DB_DSN)" up

db-down:
	goose -dir $(MIGRATIONS_DIR) mysql "$(DB_DSN)" down

db-status:
	goose -dir $(MIGRATIONS_DIR) mysql "$(DB_DSN)" status

migrate: db-up

compose-up:
	docker compose -f docker/docker-compose.yml up --build

compose-down:
	docker compose -f docker/docker-compose.yml down

compose-logs:
	docker compose -f docker/docker-compose.yml logs -f app

ci: generate fmt-check vet test cover-critical build
