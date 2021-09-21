# YDB Kubernetes Operator Helm chart

## Parameters

### Docker image configuration

| Name               | Description                               | Value                                     |
| ------------------ | ----------------------------------------- | ----------------------------------------- |
| `image.pullPolicy` | Политика скачивания образа                | `IfNotPresent`                            |
| `image.repository` | Image repository                          | `cr.yandex/crpbo4q9lbgkn85vr1rm/operator` |
| `image.tag`        | Image tag                                 | `latest`                                  |
| `imagePullSecrets` | Secrets to use for Docker registry access | `[]`                                      |


### Resource quotas

| Name                 | Description                                    | Value |
| -------------------- | ---------------------------------------------- | ----- |
| `resources.limits`   | The resource limits for Operator container     | `{}`  |
| `resources.requests` | The requested resources for Operator container | `{}`  |


