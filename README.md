[![upload-artifacts](https://github.com/ydb-platform/ydb-kubernetes-operator/actions/workflows/upload-artifacts.yml/badge.svg)](https://github.com/ydb-platform/ydb-kubernetes-operator/actions/workflows/upload-artifacts.yml)
[![compatibility-tests](https://github.com/ydb-platform/ydb-kubernetes-operator/actions/workflows/compatibility-tests.yaml/badge.svg)](https://github.com/ydb-platform/ydb-kubernetes-operator/actions/workflows/compatibility-tests.yaml)

# YDB Kubernetes Operator

The YDB Kubernetes operator deploys and manages YDB resources in a Kubernetes cluster.

## Prerequisites

1. Helm 3.1.0+
2. Kubernetes 1.20+.
3. [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)

## Limitations

- The Operator currently runs on [Amazon EKS](https://aws.amazon.com/eks/) and [Yandex Managed Service for Kubernetes®](https://cloud.yandex.com/en/services/managed-kubernetes), other cloud providers have not been tested yet.
- The Operator has not been tested with [Istio](https://istio.io/).

## Usage

For steps on how to deploy and use YDB Kubernetes Operator, please refer to [documentation](https://ydb.tech/en/docs/deploy/orchestrated/concepts).

## Development

Refer to the operator [development docs](./docs).

## Compatibility Notes

Version 0.7.0 includes major dependency upgrades that may affect compatibility with older Kubernetes clusters. The operator has been migrated from controller-runtime v0.14.1 to v0.22.4 and now requires Go 1.24 (previously 1.20). Kubernetes libraries have been updated from v0.26.15 to v0.34.x (k8s.io/api, k8s.io/apimachinery, k8s.io/client-go, k8s.io/kubectl), and k8s.io/utils has been aligned with Kubernetes 1.34 compatibility.

Due to these changes, running the operator on significantly older Kubernetes versions may lead to unexpected behavior or incompatibilities. The operator has been tested with Kubernetes v1.20 and v1.34. It is strongly recommended to use Kubernetes versions compatible with the v1.34 client libraries.
