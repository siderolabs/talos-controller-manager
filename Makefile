all: manager container

manager: generate
	CGO_ENABLED=0 go build .

.PHONY: generate
generate: # Generate code.
	controller-gen object:headerFile=./hack/boilerplate.go.txt paths="./..."

container: # Build a container image.
	docker build --tag docker.io/andrewrynhard/talos-controller-manager .

.PHONY: manifests
manifests: # Generate manifests e.g. CRD, RBAC etc.
	controller-gen rbac:roleName=talos-controller-manager-role crd paths="./..." output:rbac:artifacts:config=hack/config/rbac output:crd:artifacts:config=hack/config/crd

deploy: manifests # Deploy controller-manager to a cluster.
	kubectl apply -k hack/config

destroy: # Remove controller-manager from a cluster.
	kubectl delete -k hack/config
