SHELL := /bin/sh

REGISTRY ?= ghcr.io
GITHUB_OWNER ?= seakleangnhak
GITHUB_USERNAME ?= $(GITHUB_OWNER)
IMAGE_PREFIX ?= $(REGISTRY)/$(GITHUB_OWNER)
IMAGE_TAG ?= $(shell git rev-parse --short HEAD)

API_IMAGE ?= $(IMAGE_PREFIX)/slai-api
WEB_IMAGE ?= $(IMAGE_PREFIX)/slai-web

DOCKER_BUILD_FLAGS ?=
ATTESTATION_FLAGS ?= --provenance=false --sbom=false
PLATFORM ?=
PLATFORM_FLAG := $(if $(PLATFORM),--platform $(PLATFORM),)

.PHONY: help docker-login docker-vars docker-build docker-build-api docker-build-web docker-push docker-push-api docker-push-web docker-release

help:
	@echo "SLAI Docker targets"
	@echo ""
	@echo "  make docker-build        Build API and web images"
	@echo "  make docker-push         Push API and web images"
	@echo "  make docker-release      Build and push API and web images"
	@echo "  make docker-login        Login to GHCR using GITHUB_TOKEN"
	@echo "  make docker-vars         Print resolved image names"
	@echo ""
	@echo "Common overrides:"
	@echo "  IMAGE_TAG=v1.0.0"
	@echo "  GITHUB_OWNER=your-org"
	@echo "  REGISTRY=ghcr.io"
	@echo "  PLATFORM=linux/amd64"
	@echo "  ATTESTATION_FLAGS=\"\" # enable BuildKit provenance/SBOM"

docker-vars:
	@echo "REGISTRY=$(REGISTRY)"
	@echo "GITHUB_OWNER=$(GITHUB_OWNER)"
	@echo "IMAGE_TAG=$(IMAGE_TAG)"
	@echo "API_IMAGE=$(API_IMAGE):$(IMAGE_TAG)"
	@echo "WEB_IMAGE=$(WEB_IMAGE):$(IMAGE_TAG)"

docker-login:
	@test -n "$$GITHUB_TOKEN" || (echo "GITHUB_TOKEN is required. Create a GitHub token with write:packages." && exit 1)
	@echo "$$GITHUB_TOKEN" | docker login $(REGISTRY) -u "$(GITHUB_USERNAME)" --password-stdin

docker-build: docker-build-api docker-build-web

docker-build-api:
	BUILDX_NO_DEFAULT_ATTESTATIONS=1 docker build $(PLATFORM_FLAG) $(ATTESTATION_FLAGS) $(DOCKER_BUILD_FLAGS) -f services/api/Dockerfile -t $(API_IMAGE):$(IMAGE_TAG) -t $(API_IMAGE):latest .

docker-build-web:
	BUILDX_NO_DEFAULT_ATTESTATIONS=1 docker build $(PLATFORM_FLAG) $(ATTESTATION_FLAGS) $(DOCKER_BUILD_FLAGS) -f apps/web/Dockerfile -t $(WEB_IMAGE):$(IMAGE_TAG) -t $(WEB_IMAGE):latest .

docker-push: docker-push-api docker-push-web

docker-push-api:
	docker push $(API_IMAGE):$(IMAGE_TAG)
	docker push $(API_IMAGE):latest

docker-push-web:
	docker push $(WEB_IMAGE):$(IMAGE_TAG)
	docker push $(WEB_IMAGE):latest

docker-release: docker-build docker-push
