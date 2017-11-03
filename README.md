# etcd operator

unit/integration:
[![Build Status](https://jenkins-etcd-public.prod.coreos.systems/view/operator/job/etcd-operator-unit-master/badge/icon)](https://jenkins-etcd-public.prod.coreos.systems/view/operator/job/etcd-operator-unit-master/)
e2e (Kubernetes stable):
[![Build Status](https://jenkins-etcd-public.prod.coreos.systems/buildStatus/icon?job=etcd-operator-master)](https://jenkins-etcd-public.prod.coreos.systems/job/etcd-operator-master/)
e2e (upgrade):
[![Build Status](https://jenkins-etcd.prod.coreos.systems/buildStatus/icon?job=etcd-operator-upgrade)](https://jenkins-etcd.prod.coreos.systems/job/etcd-operator-upgrade/)

### Project status: beta

Major planned features have been completed and while no breaking API changes are currently planned, we reserve the right to address bugs and API changes in a backwards incompatible way before the project is declared stable. See [upgrade guide](./doc/user/upgrade/upgrade_guide.md) for safe upgrade process.

Currently user facing etcd cluster objects are created as [Kubernetes Custom Resources](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/), however, taking advantage of [User Aggregated API Servers](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/api-machinery/aggregated-api-servers.md) to improve reliability, validation and versioning is planned. The use of Aggregated API should be minimally disruptive to existing users but may change what Kubernetes objects are created or how users deploy the etcd operator.

We expect to consider the etcd operator stable soon; backwards incompatible changes will not be made once the project reaches stability.

## Requirements

- Kubernetes 1.7+
- etcd 3.1+

## Overview

Read *Getting Started* guides for:

- [etcd operator](doc/user/walkthrough/etcd-operator.md)
- [etcd backup operator](doc/user/walkthrough/backup-operator.md)
- [etcd restore operator](doc/user/walkthrough/restore-operator.md)

There are [more spec examples](./doc/user/spec_examples.md) on setting up clusters with backup, restore, and other configurations.

Read [Best Practices](./doc/best_practices.md) for more information on how to better use etcd operator.

Read [RBAC docs](./doc/user/rbac.md) for how to setup RBAC rules for etcd operator if RBAC is in place.

Read [Developer Guide](./doc/dev/developer_guide.md) for setting up development environment if you want to contribute.

See the [Resources and Labels](./doc/user/resource_labels.md) doc for an overview of the resources created by the etcd-operator.
