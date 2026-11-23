# Docker Desktop CLI 可能不在默认 PATH（Cursor agent shell 常见）；优先使用已安装的 Docker.app。
export PATH := /Applications/Docker.app/Contents/Resources/bin:$(PATH)

E2E_COMPOSE := docker compose -p sol-scanner-service-e2e -f scripts/test_docker_compose.yml
E2E_RUN ?= .
E2E_TIMEOUT ?= 10m
E2E_ENV := SOL_SCANNER_E2E_DSN='postgres://postgres:postgres@127.0.0.1:15442/sol_scanner_e2e?sslmode=disable' SOL_SCANNER_E2E_KAFKA=127.0.0.1:19092

.PHONY: e2e
e2e:
	@set -eu; cleanup() { $(E2E_COMPOSE) down -v --remove-orphans; }; trap cleanup EXIT; \
	cleanup; $(E2E_COMPOSE) up -d --wait; \
	$(E2E_ENV) CGO_ENABLED=1 go test -race -shuffle=on -timeout $(E2E_TIMEOUT) -tags=e2e ./internal/test/integration/... -run '$(E2E_RUN)' -count=1
