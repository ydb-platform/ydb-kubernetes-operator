## Writing tests

### Categories

Tests for the operator have naturally splitted up into three categories:

#### Small

These tests are simple unit tests that do not simulate any Kubernetes infrastructure.
Useful for testing basic logic chunks of the operator.

These tests are located directly next to the files which they are testing. Grep for
`label.go` and `label_test.go` for a simple self-contained example.

#### Medium

These tests execute use `sigs.k8s.io/controller-runtime/pkg/envtest` to spin up the
control plane of the Kubernetes cluster. This allows for testing simple interactions
between operator and Kubernetes control plane, such as expecting `StatefulSets`s and
`ConfigMap`s to be created after operator has consumed the `Storage` yaml manifest.
This would not actually spin up physical Pods though, we would test only that all the
resources have persisted in etcd! No real cluster is being created at this point.

There is no example for this one though - will be added soon!

#### End to end

These tests simulate a Kubernetes cluster using [Kind](https://kind.sigs.k8s.io/).
With this framework, it is possible to spin up many kubernetes worker nodes as docker
containers on a single machine. This allows for full-blown smoke tests - apply the
`Storage` manifests, wait until the `Pod`s become available, run `SELECT 1` inside
one of those pods to check that YDB is actually up and running!

E2E tests are located in [e2e](../e2e) folder.

## Running tests

In order to run end to end tests, you have to install `Kind`.
[Refer to official docs](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).

Typical snippet to run e2e tests follows:

```
# In case you had other cluster previously, delete it
kind delete cluster --name=local-kind

kind create cluster \
 --image=kindest/node:v1.21.14@sha256:9d9eb5fb26b4fbc0c6d95fa8c790414f9750dd583f5d7cee45d92e8c26670aa1 \
 --name=local-kind \
 --config=./e2e/kind-cluster-config.yaml \
 --wait 5m

# Switch your local kubeconfig to the newly created cluster:
kubectl config use-context kind-local-kind

# Within tests, the following two images are used:
# cr.yandex/crptqonuodf51kdj7a7d/ydb:22.4.44
# kind/ydb-operator:current

# You have to download the ydb image and build the operator image yourself. Then, explicitly 
# upload them into the kind cluster. Refer to `./github/e2e.yaml` github workflow which essentially
# does the same thing.
kind --name local-kind load docker-image kind/ydb-operator:current
kind --name local-kind load docker-image ydb:22.4.44

# Run all tests with disabled concurrency, because there is only one cluster to run tests against
go test -p 1 -v ./...
```
