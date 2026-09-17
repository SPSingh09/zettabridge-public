.PHONY: test test-unit staging-smoke staging-infra staging-deploy staging-deploy-reset staging-reset staging-deploy-full staging-release staging-release-full monitoring-up monitoring-down functional-test functional-test-billing functional-test-staging functional-test-live-routing loadtest-smoke loadtest-staging sectest-staging preflight-staging preflight-loadtest dev dev-api dev-dashboard build-dashboard build-adapters run-paper-adapter run-zerodha-adapter compose-http compose-adapters swagger

# Unit + integration tests (no docker, no real brokers)
test: test-unit

test-unit:
	go test ./...

# HTTP smoke against staging (STAGING_BASE_URL, STAGING_ADMIN_EMAIL, STAGING_ADMIN_PASSWORD)
staging-smoke:
	bash deploy/scripts/staging-smoke.sh

# AWS ECS staging smoke — loads deploy/deploy-aws-ecs/staging-aws.env, resolves api_url from terraform
staging-smoke-aws:
	bash deploy/deploy-aws-ecs/scripts/staging-smoke.sh

# AWS ECS staging — Terraform apply
staging-infra:
	bash deploy/deploy-aws-ecs/scripts/terraform-apply.sh --auto-approve

# AWS ECS staging — build/push/deploy Core + adapters + dashboard
staging-deploy:
	bash deploy/deploy-aws-ecs/scripts/aws-deploy.sh

# AWS ECS staging — stop all ECS tasks, then build/push/deploy fresh
staging-deploy-reset:
	bash deploy/deploy-aws-ecs/scripts/aws-deploy.sh --reset

# AWS ECS staging — scale all services to 0 (clears stuck deployments)
staging-reset:
	bash deploy/deploy-aws-ecs/scripts/staging-reset.sh

# AWS ECS staging — full pipeline: infra + migrate + deploy + smoke
staging-deploy-full:
	bash deploy/deploy-aws-ecs/scripts/deploy-staging-full.sh

# After git push + green GitHub Actions: deploy VM + smoke (see deploy/staging.env.example)
staging-release:
	bash deploy/scripts/staging-release.sh

staging-release-full:
	bash deploy/scripts/staging-release.sh --full

monitoring-up:
	bash deploy/scripts/monitoring-up.sh

monitoring-down:
	bash deploy/scripts/monitoring-down.sh

# Full API E2E against docker-compose (BROKER_MODE=mock)
functional-test:
	bash functional-tester/run-functional-tests.sh

# Stripe billing tests (BILLING_ENABLED overlay + signed webhooks; no Stripe CLI)
functional-test-billing:
	bash functional-tester/run-functional-billing-tests.sh

# Full functional suite against staging URL (STAGING_BASE_URL + admin creds)
functional-test-staging:
	bash functional-tester/run-functional-staging.sh

# Free plan blocked from live credentials when BROKER_MODE=live
functional-test-live-routing:
	bash functional-tester/run-functional-live-routing-test.sh

# Load test (5B.1) — requires k6 + deploy/loadtest/env.local
loadtest-smoke:
	bash deploy/loadtest/run-loadtest.sh smoke

loadtest-staging:
	bash deploy/loadtest/run-loadtest.sh $(or $(SCENARIO),target)

# Pen test (5B.2) — STAGING_BASE_URL or deploy/sectest/env.local
sectest-staging:
	bash deploy/sectest/run-sectest.sh

# Preflight before staging tests (WSL — run preflight-vm.sh on VM first)
preflight-staging:
	bash deploy/scripts/preflight-wsl.sh --require-admin

preflight-loadtest:
	bash deploy/scripts/preflight-wsl.sh --require-admin --require-loadtest

# Dashboard (4B.2) — requires Node 20+
# Run API server + dashboard concurrently; ctrl-C stops both.
dev:
	@trap 'kill 0' EXIT; \
	  go run ./cmd/api & \
	  (cd dashboard && npm run dev) & \
	  wait

dev-api:
	go run ./cmd/api

build-adapters:
	go build -o bin/paper-adapter ./cmd/paper-adapter
	go build -o bin/zerodha-adapter ./cmd/zerodha-adapter

run-paper-adapter:
	go run ./cmd/paper-adapter

run-zerodha-adapter:
	go run ./cmd/zerodha-adapter

# Phase 3: Core + paper/zerodha sidecars with EXECUTION_ADAPTER_MODE=http
compose-http:
	docker compose -f docker-compose.yml -f docker-compose.adapters.yml --profile adapters up --build

compose-adapters:
	docker compose --profile adapters up --build

dev-dashboard:
	cd dashboard && npm run dev

build-dashboard:
	cd dashboard && npm run build

# Regenerate OpenAPI spec from swaggo annotations (requires swag: go install github.com/swaggo/swag/cmd/swag@latest)
# Falls back to scripts/generate_swagger.py when swag is unavailable.
swagger:
	@if command -v swag >/dev/null 2>&1 || [ -x "$$(go env GOPATH 2>/dev/null)/bin/swag" ]; then \
	  $$(go env GOPATH)/bin/swag init \
	    -g cmd/api/docs.go \
	    -d ./ \
	    --output ./swagger \
	    --parseDependency \
	    --parseInternal && \
	  printf '//go:build ignore\n\n' | cat - swagger/docs.go > swagger/docs.go.tmp && mv swagger/docs.go.tmp swagger/docs.go; \
	else \
	  python3 scripts/generate_swagger.py; \
	fi

swagger-py:
	python3 scripts/generate_swagger.py
