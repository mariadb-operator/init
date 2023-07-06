<p align="center">
<img src="https://mariadb-operator.github.io/mariadb-operator/assets/mariadb-operator.png" alt="mariadb" width="250"/>
</p>

<p align="center">
<a href="https://github.com/mariadb-operator/init/actions/workflows/ci.yml"><img src="https://github.com/mariadb-operator/init/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://github.com/mariadb-operator/init/actions/workflows/release.yml"><img src="https://github.com/mariadb-operator/init/actions/workflows/release.yml/badge.svg" alt="Release"></a>
<a href="https://goreportcard.com/report/github.com/mariadb-operator/init"><img src="https://goreportcard.com/badge/github.com/mariadb-operator/init" alt="Go Report Card"></a>
<a href="https://pkg.go.dev/github.com/mariadb-operator/init"><img src="https://pkg.go.dev/badge/github.com/mariadb-operator/init.svg" alt="Go Reference"></a>
<a href="https://join.slack.com/t/mariadb-operator/shared_invite/zt-1xsfguxlf-dhtV6zk0HwlAh_U2iYfUxw"><img alt="Slack" src="https://img.shields.io/badge/slack-join_chat-blue?logo=Slack&label=slack&style=flat"></a>
</p>

# 🍼 init
Init container for MariaDB that co-operates with [mariadb-operator](https://github.com/mariadb-operator/mariadb-operator). Configure Galera and guarantee ordered deployments for MariaDB.
- Avoid hacking with bash `initContainers`, do it properly in Go
- Get `MariaDB` resources from the Kubernetes API and configure Galera based on them
- Guarantee MariaDB ordered deployment by checking its `Pod` Ready conditions in the Kubernetes API
- Allow `spec.podManagementPolicy` = `Parallel` in the MariaDB `StatefulSet`

### How to use it

Specify the init image in the `MariaDB` `spec.galera.initContainer` field.

```yaml
apiVersion: mariadb.mmontes.io/v1alpha1
kind: MariaDB
metadata:
  name: mariadb-galera
spec:
  ...
  image:
    repository: mariadb
    tag: "10.11.3"
    pullPolicy: IfNotPresent
  port: 3306
  replicas: 3

  galera:
    sst: mariabackup
    replicaThreads: 1

    initContainer:
      image:
        repository: ghcr.io/mariadb-operator/init
        tag: "v0.0.2"
        pullPolicy: IfNotPresent
  ...
```
