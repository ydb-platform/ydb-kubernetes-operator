# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.1.0

# Image URL to use all building/pushing image targets
IMG ?= cr.yandex/yc/ydb-operator:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.26
# Image URL which uses in tests
YDB_IMAGE ?= $(shell grep "anchor_for_fetching_image_from_workflow" ./tests/**/*.go | grep -o -E '"cr\.yandex.*"' | tr -d '"')

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	cp config/crd/bases/ydb.tech_storages.yaml deploy/ydb-operator/crds/storage.yaml
	cp config/crd/bases/ydb.tech_databases.yaml deploy/ydb-operator/crds/database.yaml
	cp config/crd/bases/ydb.tech_storagenodesets.yaml deploy/ydb-operator/crds/storagenodeset.yaml
	cp config/crd/bases/ydb.tech_databasenodesets.yaml deploy/ydb-operator/crds/databasenodeset.yaml
	cp config/crd/bases/ydb.tech_remotestoragenodesets.yaml deploy/ydb-operator/crds/remotestoragenodeset.yaml
	cp config/crd/bases/ydb.tech_remotedatabasenodesets.yaml deploy/ydb-operator/crds/remotedatabasenodeset.yaml
	cp config/crd/bases/ydb.tech_databasemonitorings.yaml deploy/ydb-operator/crds/databasemonitoring.yaml
	cp config/crd/bases/ydb.tech_storagemonitorings.yaml deploy/ydb-operator/crds/storagemonitoring.yaml

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="build/hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

kind-init:
	if kind get clusters | grep "kind-ydb-operator"; then exit 0; fi; \
	kind create cluster \
	--config tests/cfg/kind-cluster-config.yaml \
	--image=kindest/node:v1.31.2@sha256:18fbefc20a7113353c7b75b5c869d7145a6abd6269154825872dc59c1329912e \
	--name kind-ydb-operator
	docker pull k8s.gcr.io/ingress-nginx/kube-webhook-certgen:v1.0
	kind load docker-image k8s.gcr.io/ingress-nginx/kube-webhook-certgen:v1.0 --name kind-ydb-operator
	kubectl cluster-info --context kind-kind-ydb-operator # yes, kind prefixes all context with one more 'kind-'
	docker pull ${YDB_IMAGE}
	kind load docker-image ${YDB_IMAGE} --name kind-ydb-operator

kind-load:
	docker tag ${IMG} kind/ydb-operator:current
	kind load docker-image kind/ydb-operator:current --name kind-ydb-operator

opts ?= ''

.PHONY: unit-test
unit-test: manifests generate fmt vet envtest ## Run unit tests
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use --arch=amd64 $(ENVTEST_K8S_VERSION) -p path)" \
	go test -v -timeout 900s -p 1 ./internal/... -ginkgo.vv -coverprofile cover.out $(opts)

.PHONY: e2e-test
e2e-test: manifests generate fmt vet docker-build kind-init kind-load ## Run e2e tests
	go test -v -timeout 3600s -p 1 ./tests/e2e/... -ginkgo.vv $(opts)

.PHONY: test
test: unit-test e2e-test ## Run all tests

.PHONY: clean
clean:
	kind delete cluster --name kind-ydb-operator

##@ Build

build: generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/ydb-kubernetes-operator/main.go

docker-build: ## Build docker image with the manager.
	docker build --network=host -t ${IMG} .

docker-push: ## Push docker image with the manager.
	docker push ${IMG}

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
CONTROLLER_GEN_VERSION ?= v0.16.5
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION))

# ENVTEST_VERSION is usually latest, but might need to be pinned from time to time.
# Version pinning is needed due to version incompatibility between controller-runtime and setup-envtest.
# For more information: https://github.com/kubernetes-sigs/controller-runtime/issues/2744
ENVTEST = $(shell pwd)/bin/setup-envtest
ENVTEST_VERSION ?= release-0.17
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION))

# go-get-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
