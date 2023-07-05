##@ Dev

.PHONY: lint
lint: golangci-lint ## Lint.
	$(GOLANGCI_LINT) run

.PHONY: build
build: ## Build binary.
	go build -o bin/init main.go

.PHONY: test
test: ## Run tests.
	go test ./... -coverprofile cover.out

.PHONY: cover
cover: test ## Run tests and generate coverage.
	go tool cover -html=cover.out -o=cover.html

.PHONY: release
release: goreleaser ## Test release locally.
	$(GORELEASER) release --snapshot --rm-dist


.PHONY: dir
dir: ## Create config and state directories for local development.
	mkdir -p mariadb/config
	mkdir -p mariadb/state

export KUBECONFIG ?= $(HOME)/.kube/config
export POD_NAME ?= mariadb-galera-0
export MARIADB_ROOT_PASSWORD ?= mariadb
RUN_FLAGS ?= --log-dev --log-level=debug --log-time-encoder=iso8601 --mariadb-name=mariadb-galera --mariadb-namespace=default --config-dir=mariadb/config --state-dir=mariadb/state
.PHONY: run
run: dir ## Run init from your host.
	go run main.go $(RUN_FLAGS)