COMMON_ARGS = --progress=plain
COMMON_ARGS += --frontend=dockerfile.v0
COMMON_ARGS += --local context=.
COMMON_ARGS += --local dockerfile=.

SHA ?= $(shell gitmeta git sha)
TAG ?= $(shell gitmeta image tag)
BRANCH ?= $(shell gitmeta git branch)

BUILDKIT_HOST ?= tcp://0.0.0.0:1234

ifeq ($(PUSH),true)
PUSH_ARGS = ,push=true
else
PUSH_ARGS =
endif

all: manifests container

.PHONY: generate
generate: # Generate code.
	buildctl --addr $(BUILDKIT_HOST) \
		build \
		--output type=local,dest=. \
		--opt target=$@ \
		$(COMMON_ARGS)

container: generate # Build a container image.
	@mkdir -p ./build
	buildctl --addr $(BUILDKIT_HOST) \
		build \
		--output type=image,name=docker.io/autonomy/talos-controller-manager:$(TAG)$(PUSH_ARGS) \
		--opt target=$@ \
		$(COMMON_ARGS)

.PHONY: manifests
manifests: # Generate manifests e.g. CRD, RBAC etc.
	buildctl --addr $(BUILDKIT_HOST) \
		build \
		--output type=local,dest=. \
		--opt target=$@ \
		$(COMMON_ARGS)

release: manifests container
	@mkdir -p ./build
	buildctl --addr $(BUILDKIT_HOST) \
		build \
		--output type=local,dest=./build \
		--opt target=$@ \
		$(COMMON_ARGS)

deploy: manifests # Deploy to a cluster.
	kubectl apply -k hack/config

destroy: # Remove from a cluster.
	kubectl delete -k hack/config
