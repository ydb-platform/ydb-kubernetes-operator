# YDB Kubernetes Operator

The YDB Kubernetes operator deploys and manages Yandex Database resources on a Kubernetes cluster.

## Prerequisites

1. Helm 3.1.0+
2. Kubernetes 1.20+.
3. [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
4. Support for ([Dynamic Volume Provisioning](https://kubernetes.io/docs/concepts/storage/dynamic-provisioning/)).

## Limitations

- The Operator currently runs on Yandex Cloud and EKS, other cloud providers have not been tested.
- The Operator has not been tested with [Istio](https://istio.io/).

## Usage

For steps how to deploy and use YDB Kubernetes Operator, please refer to [documentation](https://cloud.yandex.ru/docs/ydb/deploy/orchestrated/yc_managed_kubernetes).

## Development

To build and test operator locally, do the following:

1. Generate CustomResourceDefinitions:
  ```bash
  make manifests
  ```
2. Install them to the cluster pointed by your current `kubeconfig`:
  ```bash
  make install
  ```
3. Run the Operator:
  ```bash
  make run
  ```
4. Build and push the Operator Docker image to the registry. Use `IMG` variable to redefine image name:
  ```bash
  IMG=cr.yandex/crpbo4q9lbgkn85vr1rm/operator:latest make docker-build docker-push
  ```