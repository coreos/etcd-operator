# Cluster TLS guide

Cluster TLS policy is configured on a per-cluster basis via the TPR spec provided to etcd-operator.

```yaml
apiVersion: "etcd.coreos.com/v1beta1"
kind: "Cluster"
metadata:
  name: "example-etcd-cluster"
spec:
  ...
  TLS:
    ....
```

For a review etcd's TLS support and requirements, please read the relevant section from the [operations guide](https://coreos.com/etcd/docs/latest/op-guide/security.html).


## Static cluster TLS Policy

```yaml
apiVersion: "etcd.coreos.com/v1beta1"
kind: "Cluster"
metadata:
  name: "example-etcd-cluster"
 spec:
  ...
  TLS:
    static:
      serverSecretName: server-tls-secret
      clientSecretName: client-tls-secret

```

* **serverSecretName**: contains pem-encoded private keys and x509 certificates needed by the etcd server.

  etcd-operator will mount this secret at `/etc/etcd-operator/server-tls` for each etcd member pod in the cluster.

  The server TLS assets are expected to conform to the following structure:

  ```text
  /etc/etcd-operator/server-tls/

       server-client-crt.pem
       server-client-key.pem
       ca-client-crt.pem

       server-peer-crt.pem
       server-peer-key.pem
       ca-peer-crt.pem
  ```

  How these files are used by the etcd server is outlined in the [security flags section of the etcd admin guide](https://github.com/coreos/etcd/blob/master/Documentation/op-guide/configuration.md#security-flags).

* **clientSecretName**: contains pem-encoded private-key and x509 certificates needed to access etcd client interface. This identity is used by `etcd-operator` and backup sidecar to access the cluster's client interface. The secret will be mounted `/etc/etcd-operator/client-tls`.

  The client TLS assets are expected to conform to the following structure:

  ```text
  /etc/etcd-operator/client-tls/

    client-crt.pem
    client-key.pem
    ca-crt.pem

  ```

  These files are similar too the `--cert-file`,`--key-file`, and `--ca-file` arguments (respectively) to `etcdctl`.
