# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.1.0

# Image URL to use all building/pushing image targets
IMG ?= cr.yandex/yc/ydb-operator:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.21

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
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
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
	kind create cluster --config e2e/kind-cluster-config.yaml --name kind-ydb-operator; \
	docker pull k8s.gcr.io/ingress-nginx/kube-webhook-certgen:v1.0; \
	kind load docker-image k8s.gcr.io/ingress-nginx/kube-webhook-certgen:v1.0 --name kind-ydb-operator; \
	docker pull cr.yandex/crptqonuodf51kdj7a7d/ydb:22.4.44; \
	kind load docker-image cr.yandex/crptqonuodf51kdj7a7d/ydb:22.4.44 --name kind-ydb-operator

kind-load:
	docker tag cr.yandex/yc/ydb-operator:latest kind/ydb-operator:current
	kind load docker-image kind/ydb-operator:current --name kind-ydb-operator

.PHONY: test
test: manifests generate fmt vet docker-build kind-init kind-load envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -timeout 1800s -p 1 ./... -ginkgo.v -coverprofile cover.out

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
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

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
