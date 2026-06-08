# Dockery root Makefile — local dev orchestration.
#
# `make dev` builds the all-in-one image from your working tree and runs
# the full stack (Web UI + dockery-api + distribution registry behind
# nginx) via docker-compose.dev.yaml. Build-and-run: re-run `make dev`
# after code changes to rebuild.

COMPOSE_DEV := docker compose -f docker-compose.dev.yaml

# First-boot admin password for the dev instance. Override on the CLI:
#   make dev DOCKERY_ADMIN_PASSWORD=hunter2
DOCKERY_ADMIN_PASSWORD ?= changeme
export DOCKERY_ADMIN_PASSWORD

.PHONY: dev
# build + start the full local dev stack (detached)
dev:
	$(COMPOSE_DEV) up --build -d
	@echo ''
	@echo 'Dockery dev is up → http://localhost:5001  (login: admin / $(DOCKERY_ADMIN_PASSWORD))'
	@echo '  logs:  make dev-logs'
	@echo '  stop:  make dev-down     (keeps data)'
	@echo '  reset: make dev-reset    (wipes the dev volume)'

.PHONY: dev-logs
# follow logs of the running dev stack
dev-logs:
	$(COMPOSE_DEV) logs -f

.PHONY: dev-down
# stop the dev stack (keeps the dev data volume)
dev-down:
	$(COMPOSE_DEV) down

.PHONY: dev-reset
# stop the dev stack AND wipe its data volume (fresh first boot)
dev-reset:
	$(COMPOSE_DEV) down -v

# show help
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
