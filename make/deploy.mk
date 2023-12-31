CLUSTER ?= mdb

##@ Docker

PLATFORM ?= linux/amd64,linux/arm64
IMG ?= ghcr.io/mariadb-operator/init:v0.0.3
BUILDX ?= docker buildx build --platform $(PLATFORM) -t $(IMG) 
BUILDER ?= init

.PHONY: docker-builder
docker-builder: ## Configure docker builder.
	docker buildx create --name $(BUILDER) --use --platform $(PLATFORM)

.PHONY: docker-build
docker-build: ## Build docker image.
	docker build -t $(IMG) .  

.PHONY: docker-buildx
docker-buildx: ## Build multi-arch docker image.
	$(BUILDX) .

.PHONY: docker-push
docker-push: ## Build multi-arch docker image and push it to the registry.
	$(BUILDX) --push .

.PHONY: docker-inspect
docker-inspect: ## Inspect docker image.
	docker buildx imagetools inspect $(IMG)

.PHONY: docker-load
docker-load: ## Load docker image in KIND.
	kind load docker-image --name ${CLUSTER} ${IMG}

##@ Cluster

.PHONY: cluster-ctx
cluster-ctx: ## Sets cluster context.
	@kubectl config use-context kind-$(CLUSTER)
