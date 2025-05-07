# Changelog


## v0.6.3 - 2025-05-07

## v0.6.2 - 2025-02-24
### Fixed
* bug: regression with pod name in grpc-public-host arg

## v0.6.1 - 2025-02-12
### Fixed
* fix passing interconnet TLS volume in blobstorage-init job

## v0.6.0 - 2025-01-29
### Added
* starting with this release, deploying to dockerhub (ydbplatform/ydb-kubernetes-operator)
* added the ability to create metadata announce for customize dns domain (default: cluster.local)
* new field additionalPodLabels for Storage and Database CRD
* new method buildPodTemplateLabels to append additionalPodLabels for statefulset builders
* compatibility tests running automatically on each new tag
* customize Database and Storage container securityContext
* field externalPort for grpc service to override --grpc-public-port arg
* annotations overrides default secret name and key for arg --auth-token-file
* field ObservedGeneration inside .status.conditions
### Changed
* up CONTROLLER_GEN_VERSION to 0.16.5 and ENVTEST_VERSION to release-0.17
* refactor package labels to separate methods buildLabels, buildSelectorLabels and buildeNodeSetLabels for each resource
* propagate labels ydb.tech/database-nodeset, ydb.tech/storage-nodeset and ydb.tech/remote-cluster with method makeCommonLabels between resource recasting
### Fixed
* e2e tests and unit tests flapped because of the race between storage finalizers and uninstalling operator helm chart
* regenerate CRDs in upload-artifacts workflow (as opposed to manually)
* additional kind worker to maintain affinity rules for blobstorage init job
* update the Makefile with the changes in GitHub CI
* bug: missing error handler for arg --auth-token-file
* fix field resourceVersion inside .status.remoteResources.conditions
* panic when create object with .spec.pause is true
* Passing additional secret volumes to blobstorage-init. The init container can now use them without issues.
### Security
* bump golang-jwt to v4.5.1 (by dependabot)
* bump golang.org/x/net from 0.23.0 to 0.33.0 (by dependabot)

## v0.5.32 - 2024-11-05
### Fixed
* Chart.yaml version is bumped up automatically when a new release PR is created

## v0.5.31 - 2024-11-04
### Added
* Initialized a changelog
