# Use bash as shell
SHELL = /bin/bash

# --- Variable Definitions ---
IMAGE_NAME   ?= inferno
REGISTRY     ?= quay.io/kuadrant
GIT_COMMIT   ?= $(shell git rev-parse --short HEAD)
IMAGE_TAG    ?= $(GIT_COMMIT) # Default tag to git commit hash
LATEST_TAG   ?= latest

IMAGE_REF    = $(REGISTRY)/$(IMAGE_NAME)

CONTAINER_TOOL ?= podman

# --- Go Module Maintenance & Testing ---
.PHONY: tidy
tidy:
	@echo "Tidying and formatting..."
	go mod tidy
	go fmt ./...

.PHONY: verify
verify:
	@echo "Verifying modules..."
	go mod verify

mod: tidy verify

.PHONY: test
test:
	@echo "Running unit tests..."
	go test -v ./...

# --- Building/Releasing Image ---
.PHONY: build
build:
	@echo "Building image using $(CONTAINER_TOOL): $(IMAGE_REF):$(IMAGE_TAG)..."
	$(CONTAINER_TOOL) build \
		--tag $(IMAGE_REF):$(IMAGE_TAG) .
	@echo "Image build complete."

.PHONY: push
push:
	@echo "Pushing image using $(CONTAINER_TOOL): $(IMAGE_REF):$(IMAGE_TAG)..."
	$(CONTAINER_TOOL) push $(IMAGE_REF):$(IMAGE_TAG)

release: build push
	@echo "Image $(IMAGE_REF):$(IMAGE_TAG) built and pushed."

