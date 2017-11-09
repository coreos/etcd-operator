# Dynamic TLS Certs

Authors: Eric Chiang(eric.chiang@coreos.com), Hongchao Deng(hongchao.deng@coreos.com)

## Background

Motivations:
- Simplify TLS cert generation for etcd cluster.
- Self-hosted etcd wants to depend on Node IP, not hostnames.

Requirements:
- Dynamically generate certs per etcd pod
- Able to use host IP in self hosted.


## Proposed solution

### Design

> For initial design, we focus on self-hosted etcd use case, although the same methodology applies to app etcd.

[Kubernetes certificates API][k8s_csr] provides the ability to send and approve CSR.
Each etcd pod will request certs via CSR API based on its hostname/IP.

There is a [ongoing effort][auto_tls] on upstream to develop an auto-approver for self-hosted k8s components.
We can make use of that instead of creating our own.

Workflow:
- etcd operator creates an etcd pod.
  A service account that has access to CSR API should be assigned to each etcd pod.
- etcd pod first runs an init container to send a CSR and wait for approval.
- An external process should verify the CSR and approve it if correct.
- Once approved, etcd pod gets the certs and bootstraps.

Pros:
- Hooks into existing k8s.
- Delegates CA signing cert management. Not touching the signing certs and keys directly.

Cons:
- Share the same CA signing cert + key as k8s.
- Requires etcd to be trusted by k8s cluster CA (probably only appropriate for self-hosted etcd).
- Requires additional approver setup.



### API Changes

Add DynamicTLS into existing TLSPolicy:

```Go
type TLSPolicy struct {
	// DynamicTLS sets how to generate TLS certs dynamically.
	Dynamic *DynamicTLS
}

type DynamicTLS struct {
  // EtcdPodServiceAccount is the service account used by each etcd pod to send CSR to APIServer.
	EtcdPodServiceAccount string
}
```

### Implementation and testing plan

We will first implement self-hosted case, and then normal app case.
Because we will have self-hosted user scenario.
For self-hosted case, we will implement dynamic peer TLS first.
Then dependence on hostnames should be gone and kubeadm can be unblocked in prototyping.

Implementation plan:

1. Change self-hosted etcd to use pod IP for server and peer URL.
2. Implement dynamic peer TLS for self-hosted etcd.
   We will still use static TLS for client, server certs initially.

TODO: We focus on self-hosted use case initially. We will add more on normal app case later.

Testing plan:

- Once step 2 is finished, add a test for self-hosted case.


## Alternatives

### Option 2: CA signing cert + key, etcd operator generates certs

etcd operator generates certs for a new member and puts them into a secret.
Then etcd operator creates an etcd pod mounted with the secret.

Workflow:
- CA signing cert + key are provided by user in a secret.
- Before creating etcd pod, etcd operator generates etcd server + peer certs, stores the certs in a secret.
  When creating etcd pod, the TLS secret is mounted as a volume.

Pros:
- Each etcd cluster has its own CA.

Cons:
- Hard for self-hosted etcd because the etcd-operator doesn't schedule pods directly onto nodes,
  so it doesn't know what serving address to sign ahead of time.
  This might be good for application etcd which uses hostnames known for each member.


## Appendix

- [Support using Kubernetes CA for certificates](https://github.com/coreos/etcd-operator/issues/1465)
- [Allow members to advertise using IPs ](https://github.com/coreos/etcd-operator/issues/1617)
- [etcd: Require a specific cert CN from peer client certificate](https://github.com/coreos/etcd/issues/8262)
- [bootkube: auto-approval for kubelet](https://github.com/kubernetes-incubator/bootkube/pull/663)



[k8s_csr]:https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/#requesting-a-certificate
[auto_tls]:https://docs.google.com/document/d/1POXVGyEoySvSnx_OftQ2CIWM0HCk27j2VZSOR4XVCDg/edit#heading=h.e742mn9kyevr