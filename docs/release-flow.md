## Release flow

#### How and when the operator version is changed

The single source of truth is the version number in
[Chart.yaml](https://github.com/ydb-platform/ydb-kubernetes-operator/blob/master/deploy/ydb-operator/Chart.yaml#L18)
file.

It is incremented according to semver practices. Essentially, it is incremented every
time any change is made into either chart or the operator code.

For the contrast, changing some details of Github workflows or rewriting the docs
does not initiate a new release.

#### What the CI does

When you increment the version in `Chart.yaml` and your PR is merged into master, tag
is automatically extracted from the Chart like this:

```
"version: 0.4.22"  ->  "0.4.22"
```

If the version is new (no previous commit was tagged with this version), then:

- this merge commit gets tagged with version extracted from `Chart.yaml`;
- new docker image is built and uploaded to `cr.yandex/yc/ydb-kubernetes-operator`;
- new chart version is uploaded to https://charts.ydb.tech.

#### What if I forget to bump up the chart version?

Then these changes won't get automatically tagged, new version won't be built and
uploaded and you most likely will notice it immediately when you'll want to use an
updated version. Then you make another PR with a chart version bump and get your
changes rebuilt.
