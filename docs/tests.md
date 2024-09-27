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

These tests are also located directly next to the files which they are testing. Grep
for `storage/controller_test.go` for an example.

##### !! Warning !! while writing medium tests.

Since `StatefulSet` controller is NOT running (only apiserver and etcd are running in
this lightweight scenario), no `Pod`s will be created and it is useless to try to
`get` pods within such tests. Only the objects that are directly created by the
operator (`StatefulSet`s, `ConfigMap`s and `Secret`s) will be created. If you want to
test some changes in `Pod` template, the correct way is to get the `StatefulSet`
object and query it's `Spec.Template.WhateverYouNeed` field to see the changes
reflected in the pod template of `StatefulSet` itself. Again, refer to
`storage/controller_test.go` for an example.

#### End to end

These tests simulate a Kubernetes cluster using [Kind](https://kind.sigs.k8s.io/).
With this framework, it is possible to spin up many kubernetes worker nodes as docker
containers on a single machine. This allows for full-blown smoke tests - apply the
`Storage` manifests, wait until the `Pod`s become available, run `SELECT 1` inside
one of those pods to check that YDB is actually up and running!

E2E tests are located in [e2e](../e2e) folder.

## Running tests

Currently we run all the tests together all the time, we'll add a snippet on how to
launch tests of fixed size later!

#### Prerequisites for medium

Run the following (from the root of the repository):

```
make envtest
./bin/setup-envtest use 1.26
echo $KUBEBUILDER_ASSETS
```

If you're on Linux this variable should look like this:
`/path/to/.local/share/kubebuilder-envtest/k8s/1.26.1-linux-amd64`. If you're on Mac,
it's probably something similar.

This snippet will install kube-apiserver and etcd binaries and put them in a location
which the testing framework knows about and will find the binaries during tests.

#### Prerequisites for end to end

In order to run end to end tests, you have to install `Kind`.
[Refer to official docs](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).

Typical snippet to run e2e tests follows:

```bash
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
# cr.yandex/crptqonuodf51kdj7a7d/ydb:<version>
# kind/ydb-operator:current

# You have to download the ydb image and build the operator image yourself. Then, explicitly
# upload them into the kind cluster. Refer to `./github/e2e.yaml` github workflow which essentially
# does the same thing.
kind --name local-kind load docker-image kind/ydb-operator:current
kind --name local-kind load docker-image ydb:<version>

# Run all tests with disabled concurrency, because there is only one cluster to run tests against
go test -p 1 -v ./...
```
