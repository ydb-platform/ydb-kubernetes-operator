## Build and test

To build and test operator locally, do the following:

0. Make sure you have `go` 1.19 installed

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