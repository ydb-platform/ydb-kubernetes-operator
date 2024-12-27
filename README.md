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
