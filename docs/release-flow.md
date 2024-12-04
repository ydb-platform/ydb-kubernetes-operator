## Release flow

#### How and when the operator version is changed

The single source of truth is the changelog. 

Currently, version is updated when a developer decides to release a new version by manually invoking
`create-release-pr` workflow in Github Actions:

- invoke `create-release-pr` workflow
- `changie` tool automatically bumps the version and generates an updated CHANGELOG.md
- if a generated `CHANGELOG.md` looks okay, just merge it
- the `upload-artifacts` workflow will:
    - substitute the latest version in `Chart.yaml`
    - build artifacts (docker image and helm chart) and upload them to all configured registries
    - create a new Github release 
