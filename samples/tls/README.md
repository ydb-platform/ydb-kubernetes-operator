This example shows how to deploy YDB with TLS enabled on both `Storage` and `Database` ends.

### Prerequisites

The following components should be present and set up in the cluster beforehand.

1. [cert-manager](https://github.com/cert-manager/cert-manager)
2. [external-dns](https://github.com/kubernetes-sigs/external-dns)

### Deploy guide

1. Create [`Issuer`](https://cert-manager.io/docs/concepts/issuer/) which can issue trusted certificates, if you don't
   have one already.
2. Update hostnames to the desired ones in the following files:
   1. `grpc-storage-crt.yaml`
   2. `grpc-database-crt.yaml`
   3. `storage.yaml`
   4. `database.yaml`
3. Update hostname in the `storage.yaml` manifest
4. Apply the manifests and wait for `Storage` to become ready
    ```
    kubectl apply -f selfsigned-issuer.yaml
    kubectl apply -f interconnect-crt.yaml
    kubectl apply -f grpc-storage-crt.yaml
    kubectl apply -f storage.yaml
    kubectl get storages.ydb.tech --watch
    ```
5. Apply the manifests and wait for the `Database` to become ready
    ```
    kubectl apply -f grpc-database-crt.yaml
    kubectl apply -f database.yaml
    kubectl get databases.ydb.tech --watch
    ```
6. Test that YDB is ready for TLS connections
    ```
    kubectl port-forward storage-sample-0 2135
    openssl s_client -showcerts -connect localhost:2135 </dev/null
    ```