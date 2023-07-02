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

export KUBECONFIG ?= $(HOME)/.kube/config
RUN_FLAGS ?= --log-dev --log-level=debug --log-time-encoder=iso8601 --mariadb-name=mariadb-galera --mariadb-namespace=default --config-dir=mariadb/config --state-dir=mariadb/state
.PHONY: run
run: ## Run init from your host.
	go run main.go $(RUN_FLAGS)