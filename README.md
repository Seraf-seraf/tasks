# Task Service

REST API сервис для управления задачами в командах. Сервис поддерживает регистрацию и JWT-аутентификацию, роли в командах, историю изменений задач, Redis-кеширование списков задач и отчетные SQL-запросы.

## Стек

- Go
- MySQL 8
- Redis
- Docker Compose
- goose
- slog
- testify
- testcontainers
- Prometheus

## API

Документация OpenAPI находится в [docs/openapi.yaml](docs/openapi.yaml).
Go-модели и chi handlers генерируются из этой спецификации через `oapi-codegen`.

Основные эндпоинты:

- `POST /api/v1/register` - регистрация пользователя.
- `POST /api/v1/login` - аутентификация и получение JWT.
- `POST /api/v1/teams` - создание команды.
- `GET /api/v1/teams` - список команд текущего пользователя.
- `POST /api/v1/teams/{id}/invite` - приглашение пользователя в команду.
- `POST /api/v1/tasks` - создание задачи.
- `GET /api/v1/tasks` - список задач с фильтрацией и пагинацией.
- `PUT /api/v1/tasks/{id}` - обновление задачи.
- `GET /api/v1/tasks/{id}/history` - история изменений задачи.
- `GET /api/v1/reports` - отчетные SQL-запросы.
- `GET /metrics` - Prometheus метрики.

Защищенные эндпоинты используют заголовок:

```text
Authorization: Bearer <jwt>
```

## Генерация кода

Сгенерировать модели, strict server interface и chi routing glue:

```bash
make generate
```

Конфигурация генератора находится в [config/oapi-codegen.yaml](config/oapi-codegen.yaml), результат генерации - в `internal/httpapi/openapi.gen.go`.

## Конфигурация

`make run` запускает приложение с [config/config.yaml](config/config.yaml). Значения можно переопределить переменными окружения.
Пример конфигурации находится в [config/config.example.yaml](config/config.example.yaml).
Для локальной разработки устанавливать переменные окружения необязательно. Пример переопределений находится в [.env.example](.env.example).

- `HTTP_ADDR`
- `MYSQL_DSN`
- `REDIS_ADDR`
- `REDIS_DB`
- `JWT_SECRET`

Пример локального MySQL DSN:

```text
task:task@tcp(localhost:3306)/tasks?parseTime=true&multiStatements=true
```

## Запуск

Поднять MySQL, Redis и приложение:

```bash
make compose-up
```

Docker-файлы находятся в [docker](docker). Makefile запускает compose через `docker/docker-compose.yml`.

Запустить приложение локально:

```bash
make run
```

Для локального запуска нужен доступный MySQL на `localhost:3306` и Redis на `localhost:6379`; эти значения уже указаны в [config/config.example.yaml](config/config.example.yaml).

Настройка конфигурации:

```bash
cp config/config.example.yaml config/config.yaml
cp .env.example .env
```

Собрать бинарник:

```bash
make build
```

Остановить docker-compose:

```bash
make compose-down
```

## Миграции

Миграции находятся в [internal/migrations](internal/migrations).

Применить миграции:

```bash
make db-up
```

Откатить последнюю миграцию:

```bash
make db-down
```

Проверить статус миграций:

```bash
make db-status
```

Для подключения к другой базе передайте `MYSQL_DSN`:

```bash
make db-up MYSQL_DSN='user:pass@tcp(localhost:3306)/tasks?parseTime=true&multiStatements=true'
```

## Тесты

Unit-тесты:

```bash
make test
```

Интеграционные тесты с MySQL testcontainers:

```bash
make test-integration
```

Покрытие:

```bash
make cover
```

Полный локальный прогон:

```bash
make ci
```

## Отчеты

Эндпоинт `GET /api/v1/reports` возвращает:

- статистику по командам: название, количество участников, количество задач `done` за последние 7 дней;
- топ-3 пользователей по количеству созданных задач в каждой команде за месяц;
- задачи, где назначенный пользователь не является участником команды задачи.
