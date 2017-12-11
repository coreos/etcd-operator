# etcd cluster TLS

## Abstract

The primary goals etcd-operator cluster TLS:
 * Encrypt etcd client/peer communication
 * Cryptographically attestable identities for following components:
    * etcd operator
    * etcd cluster TPR objects
    * backup sidecar pods
    * etcd pods
 * Cryptographically enforced cluster isolation (backup pod from cluster A CANNOT possibly talk to etcd pod in cluster B)

## Intra-Cluster PKI overview

Here is the overview for etcd-operator TLS flow, which should set us up well for integrating with pre-existing external PKI.

### Trust delegation diagram:

```
    -----------------
    | external PKI  |  (something that can sign the operator's CSRs)
    -----------------
          |     |
          |  | /|\ CERT
          |  |  |   |
          |  |  |   |
          |  | CSR \|/
          |  |
          |  |
          |  |
          |  |---> [ operator client-interface CA ]
          |         |
          |            |                    --------------> [ etcd-cluster-A client-interface CLIENT CERT ]
          |            |                    |
          |            |             DIRECT | SIGN
          |            |                     |
          |            |-------->  [ etcd-cluster-A client-interface CERTIFICATE AUTHORITY ]
          |            |                    |
          |         D  |                    |  etcd-cluster-A-0000
          |         I  |                    |-------------> [ client-interface SERVER CERT ]
          |         R  |           /|\ CERT |
          |         E  |            |   |   |  etcd-cluster-A-0001
          |         C  |           CSR \|/  |-------------> [ client-interface SERVER CERT ]
          |         T  |                    |
          |            |                    |  etcd-cluster-A-0002
          |         S  |                    |-------------> [ client-interface SERVER CERT ]
          |         I  |                    |
          |         G  |                    |  cluster-A-backup-sidecar
          |         N  |                    |-------------> [ client-interface CLIENT CERT ]
          |            |                    |
          |            |                    |
          |            |                    |
          |            |                    --------------> [ etcd-cluster-B client-interface CLIENT CERT ]
          |            |                    |
          |            |             DIRECT | SIGN
          |            |                    |
          |            |                    |-------->  [ etcd-cluster-B client-interface CERTIFICATE AUTHORITY ]
     /|\  | CERT       |                    |
      |   |  |         |                    |  etcd-cluster-B-0000
      |      |         |                    |------> ...
     CSR  | \|/        |
          |            | ...
          |
          |
          |------> [ operator peer-interface CA ]
                    |
                    |
                    |-------->  [ etcd-cluster-A peer interface CERTIFICATE AUTHORITY ]
                    |                    |
                 D  |                    |  etcd-cluster-A-0000
                 I  |                    |-------------> [ peer-interface SERVER CERT ]
                 R  |           /|\ CERT |
                 E  |            |   |   |  etcd-cluster-A-0001
                 C  |           CSR \|/  |-------------> [ peer-interface SERVER CERT ]
                 T  |                    |
                    |                    |  etcd-cluster-A-0002
                 S  |                    |-------------> [ peer-interface SERVER CERT ]
                 I  |
                 G  |
                 N  |
                    |-------->  [ etcd-cluster-B peer interface CERTIFICATE AUTHORITY ]
                    |                     |
                    |                     |  etcd-cluster-B-0000
                    |                     |------> ...
                    |
                    | ...
```



### Certificate signing procedure

1. etcd-operator pod startup:
  * generate `operator CA` private key
  * generate `operator CA` certificate (peer and client) (select one of following)
      * generate self-signed cert (default for now, useful for development mode)
      * generate CSR, wait for external entity to sign it and return cert via Kubernetes API (production mode, allows integration with arbitrary external PKI systems)

2. etcd cluster creation (in operator pod):
  * generate private key
  * generate `operator CA` as a subordinate CA of `cluster CA` using parameter from cluster spec

3. etcd node pod startup (in the etcd pod, prior to etcd application starting):
  * generate private key
  * generate a CSR for `CN=etcd-cluster-xxxx`, submit for signing via annotation  (peer and client)

4. etcd node enrollment (in operator pod) (peer and client)
  * observe new CSR annotation on `pod/etcd-cluster-xxxx`
  * sign CSR with the `cluster CA` for `pod/etcd-cluster-xxxx`
  * --> return certificate via annotation to `pod/etcd-cluster-xxxx`

5. etcd node startup (in etcd pod)
  * observe new signed certificate in annotation on `pod/etcd-cluster-xxxx`  (peer and client)
  * bring up etcd application with private key, peer cert, client cert, and `cluster CA` certificate

### Signing Mechanisms

#### Direct Sign (intra-component)

```
           [ signer ]
               |
               |
        DIRECT | SIGN
               |
               |
               -----> [ signee ]
```

In the case that the _signer_ and _signee_ are within the same component, we have the _signer's_ private key material immediately available to produce a signed certificate for the _signee_. No need for CSR exchange.

#### CSR/Cert exchange (inter-component)

```
           [ signer ]
               |
     /|\ CERT  |
      |   |    |
      |   |    |
     CSR \|/   |
               -----> [ signee ]
```


This is a symbol for a _signee_ submitting a CSR to a _signer_, and receiving back a signed certificate back.

In the case of etcd-operator, this will be coordinated via the Kubernetes API server.

-----

Here are the steps:

1. _signer_ schedules _signee_ pod to the Kubernetes cluster

2. _signer_ sets up a watch on _signee_

3. _singee_ sets up a watch on _signer_

4. _signee_ generates a private key and then a CSR, containing desired metadata

5. _signee_ annotates itself with `kubeEtcdCSR=<CSR-base64-bytes>`

6. _singer_ sees `kubeEtcdCSR` annotation, verifies metadata, and generates a signed certificate from the incoming CSR

7. _signer_ annotates _signee_ pod with `kubeEtcdCert=<cert-base64-bytes`

8. _signee_ processes `kubeEtcdCert` annotation and writes incoming cert to file system

9. _signee_ launches application

_note: steps 2-9 can be repeated to implement a primitive cert refresh mechanism_

------


Here's a table showing how this process is currently used in the etcd operator TLS infrastructure:

| _signer_      | signing CA     |  _singee_  | signed identities  | identity type |
| ------------- | -------------- | ---------- | ----------------- | ------------- |
| external PKI  | external CA    | operator | <ul><li>operator peer CA</li><li>operator client CA</li></ul> | CERTIFICATE AUTHORITY |
| operator      | clusterA peer CA | etcd-xxxx | etcd-xxxx-peer-interface | SERVER CERTIFICATE |
| operator      | clusterA client CA | etcd-xxxx | etcd-xxxx, client-interface | SERVER CERTIFICATE  |
| operator      | clusterA client CA | clusterA-backup-sidecar | clusterA-backup-sidecar, client CA | CLIENT CERTIFICATE |

## Things to note:
* **Private Keys Stay Put:** If a private key is needed, is its generated on and never leaves the component that uses it. The business of shuffling around private key material across networks is a dangerous business.

  Most importantly, the external PKI component must be allowed to sign the operator's CSR _without_ divulging it's CA private key to the cluster.

* **Separate peer and client cert chains:** The motivation is to provide the ability to isolate the etcd peer (data) plane from the etcd client (control) plane.

  The client interface CA will be expected to sign CSRs for any new entity that wants to "talk to" the cluster- this includes entirely external components like backup operators, load-balancers, etc.

  The peer interface CA, on the other hand, will sign CSRs only for new entities that want to join the cluster.

* **CSR attestation:** As of now, there has been discussion (but no planning done) towards providing a discrete mechanism for a  _signer_ to verify that an incoming CSR is actually from the claimed _signee_ before producing a certificate.

  It's unclear what the role or scope of such a mechanism should be, in light of the fact that the CSR metadata is already tied to a Kubernetes object.

  In theory, Kubernetes-provided isolation mechanisms alone should allow a _signer_ to:
   * create a _signee_ pod
   * observe an `kubeEtcdCSR` annotation on that _signee_ pod
   * be guaranteed that the _signee_ pod itself annotated that CSR.



