## Writing tests

### Categories

Tests for the operator have naturally splitted up into three categories:

#### Small

These tests are simple unit tests that do not simulate any Kubernetes infrastructure.
Useful for testing basic logic chunks of the operator. 

These tests are located directly next to the files which they are testing. Grep for `label.go` and 
`label_test.go` for a simple self-contained example.

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

