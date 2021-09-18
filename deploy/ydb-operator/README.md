# YDB Operator

[YDB](https://cloud.yandex.ru/services/ydb) — распределённая NewSQL СУБД с поддержкой бессерверных вычислений. YDB
сочетает высокую доступность и масштабируемость с поддержкой строгой консистентности и ACID-транзакций.

## TL;DR

```bash
# helm repo add yandex-cloud
# helm repo update
$ git clone https://github.com/yandex-cloud/ydb
$ helm install my-release ./ydb
```

## Введение

Чарт инициализирует кластер [YDB](https://cloud.yandex.ru/services/ydb) в кластере 
[Kubernetes](http://kubernetes.io) при помощи пакетного менеджера [Helm](https://helm.sh).

## Для установки необходимы

- Kubernetes 1.20+
- Helm 3.1.0
- Поддержка PV provisioning в кластере

## Установка

```bash
# helm repo add yandex-cloud
# helm repo update
$ git clone https://github.com/yandex-cloud/ydb
$ helm install my-release ./ydb
```

Перечислить установленные релизы можно выполнив `helm list`

## Удаление

Для удаления релиза `ydb`:

```bash
$ helm delete ydb
$ kubectl get delete -l app.kubernetes.io/instance=ydb
```

Данные команды удаляют все установленные чартом компоненты.

## Параметры

### Конфигурация Docker образов YDB

| Name                       | Description                               | Value                                |
| -------------------------- | ----------------------------------------- | ------------------------------------ |
| `image.dynamic.pullPolicy` | Политика скачивания образа                | `IfNotPresent`                       |
| `image.dynamic.repository` | Репозиторий образа                        | `cr.yandex/crpbo4q9lbgkn85vr1rm/ydb` |
| `image.dynamic.tag`        | Тэг образа                                | `986960060.trunk.hardening`          |
| `image.init.pullPolicy`    | Политика скачивания образа                | `IfNotPresent`                       |
| `image.init.repository`    | Репозиторий образа                        | `cr.yandex/crpbo4q9lbgkn85vr1rm/ydb` |
| `image.init.tag`           | Тэг образа                                | `986960060.trunk.hardening`          |
| `image.storage.pullPolicy` | Политика скачивания образа                | `IfNotPresent`                       |
| `image.storage.repository` | Репозиторий образа                        | `cr.yandex/crpbo4q9lbgkn85vr1rm/ydb` |
| `image.storage.tag`        | Тэг образа                                | `986960060.trunk.hardening`          |
| `imagePullSecrets`         | Перечисление секретов Docker репозиториев | `[]`                                 |


### Конфигурация нод хранения  # TODO better naming for storage nodes?

| Name                   | Description                                                                    | Value       |
| ---------------------- | ------------------------------------------------------------------------------ | ----------- |
| `storage.diskSize`     | Объём PVC для нод хранения                                                     | `80Gi`      |
| `storage.replicas`     | Количество запущенных нод хранения                                             | `8`         |
| `storage.publicHost`   | (опционально) FQDN, который анонсируется клиентам при первом подключении к YDB | `localhost` |
| `storage.affinity`     | Affinity for YDB pods assignment                                               | `{}`        |
| `storage.nodeSelector` | Node labels for YDB pods assignment                                            | `{}`        |
| `storage.tolerations`  | Tolerations for YDB pods assignment                                            | `[]`        |


### Конфигурация динамических нод

| Name                   | Description                                     | Value |
| ---------------------- | ----------------------------------------------- | ----- |
| `dynamic.replicas`     | Количество запущенных подов для каждой БД       | `8`   |
| `dynamic.secrets`      | Секреты, предоставляемые динамическим нодам YDB | `{}`  |
| `dynamic.tenant`       | Список активных БД  # FIXME list                | `{}`  |
| `dynamic.affinity`     | Affinity for YDB pods assignment                | `{}`  |
| `dynamic.nodeSelector` | Node labels for YDB pods assignment             | `{}`  |
| `dynamic.tolerations`  | Tolerations for YDB pods assignment             | `[]`  |


### Конфигурация метрик

| Name                  | Description                                                                     | Value   |
| --------------------- | ------------------------------------------------------------------------------- | ------- |
| `prometheus.enabled`  | Включает поддержку Prometheus метрик                                            | `false` |
| `prometheus.services` | Список внутренних сервисов YDB для которых будут созданы ресурсы ServiceMonitor | `[]`    |


### Конфигурация сервиса

| Name                                            | Description                                                 | Value       |
| ----------------------------------------------- | ----------------------------------------------------------- | ----------- |
| `serviceAccount.create`                         | Specifies whether a service account should be created       | `true`      |
| `serviceAccount.annotations`                    | Annotations to add to the service account                   | `{}`        |
| `serviceAccount.name`                           | The name of the service account to use.                     | `""`        |
| `service.common.ports.interconnect.port`        | Номер порта                                                 | `19001`     |
| `service.common.ports.interconnect.annotations` | Дополнительные аннотации                                    | `{}`        |
| `service.common.ports.interconnect.type`        | Тип порта                                                   | `ClusterIP` |
| `service.common.ports.interconnect.labels`      | Дополнительные метки                                        | `{}`        |
| `service.storage.selectors`                     | Дополнительные селекторы для всех сервисов нод хранения     | `{}`        |
| `service.storage.ports.grpc.type`               | Тип порта                                                   | `ClusterIP` |
| `service.storage.ports.grpc.port`               | Номер порта                                                 | `2135`      |
| `service.storage.ports.grpc.annotations`        | Дополнительные аннотации                                    | `{}`        |
| `service.storage.ports.grpc.labels`             | Дополнительные метки                                        | `{}`        |
| `service.storage.ports.status.type`             | Тип порта                                                   | `ClusterIP` |
| `service.storage.ports.status.port`             | Номер порта                                                 | `8765`      |
| `service.storage.ports.status.annotations`      | Дополнительные аннотации                                    | `{}`        |
| `service.storage.ports.status.labels`           | Дополнительные метки                                        | `{}`        |
| `service.dynamic.selectors`                     | Дополнительные селекторы для всех сервисов динамических нод | `{}`        |
| `service.dynamic.ports.grpc.type`               | Тип порта                                                   | `ClusterIP` |
| `service.dynamic.ports.grpc.port`               | Номер порта                                                 | `2135`      |
| `service.dynamic.ports.grpc.annotations`        | Дополнительные аннотации                                    | `{}`        |
| `service.dynamic.ports.grpc.labels`             | Дополнительные метки                                        | `{}`        |


### Конфигурация приложения

| Name          | Description                  | Value |
| ------------- | ---------------------------- | ----- |
| `config.init` | Конфигурация init контейнера | `{}`  |
| `config.ydb`  | Конфигурация YDB             | `{}`  |


### Конфигурация выделяемых ресурсов

| Name                         | Description                               | Value |
| ---------------------------- | ----------------------------------------- | ----- |
| `resources.storage.limits`   | Ограничения ресурсов для нод хранения     | `{}`  |
| `resources.storage.requests` | Необходимые ресурсы для нод хранения      | `{}`  |
| `resources.dynamic.limits`   | Ограничения ресурсов для динамических нод | `{}`  |
| `resources.dynamic.requests` | Необходимые ресурсы для динамических нод  | `{}`  |


