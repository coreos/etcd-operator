# Setting up RBAC for etcd operator

If RBAC is in place, users must create RBAC rules for etcd operator. This doc serves a tutorial for it.

## Production setup

In production, allow access only to the resources etcd operator needs, and create a specific role for the operator.

The following example binds a role to the `default` service account in the namespace in which the etcd operator is running. To bind to a different service account, modify the `subjects.name` field in the [rolebinding templates][rbac-templates] as needed.

### Role vs ClusterRole

The permission model required for the etcd operator depends on the value of its `--create-crd` flag:
- `--create-crd=true`: Creates a CRD if one does not yet exist. This the default behavior.
  - In this mode the operator requires a ClusterRole with the permission to create a CRD.
- `--create-crd=false` Creates a CR without first creating a CRD.
  - In this mode the operator can be run with just a Role without the permission to create a CRD.

## Set up RBAC

Set up RBAC rules using either a ClusterRole or Role, according to the `--create-crd` flag requirements listed above.

Modify and export the following environment variables. These will be used to fill out the [RBAC templates][rbac-templates]:

```
export ROLE_NAME=<role-name>
export ROLE_BINDING_NAME=<role-binding-name>
export NAMESPACE=<namespace>
```

### RBAC with ClusterRole (create-crd=true)

1. Create a ClusterRole:

    ```sh
    sed -e "s/<ROLE_NAME>/${ROLE_NAME}/g" example/rbac/cluster-role-template.yaml \
      | kubectl create -f -
    ```

2. Create a ClusterRoleBinding which binds the default service account in the namespace to the ClusterRole:

    ```sh
    sed -e "s/<ROLE_NAME>/${ROLE_NAME}/g" \
      -e "s/<ROLE_BINDING_NAME>/${ROLE_BINDING_NAME}/g" \
      -e "s/<NAMESPACE>/${NAMESPACE}/g" \
      example/rbac/cluster-role-binding-template.yaml \
      | kubectl create -f -
    ```

### RBAC with Role (create-crd=false)

1. Create a Role:

    ```sh
    sed -e "s/<ROLE_NAME>/${ROLE_NAME}/g" \
      -e "s/<NAMESPACE>/${NAMESPACE}/g" \
      example/rbac/role-template.yaml \
      | kubectl create -f -
    ```

2. Create a RoleBinding which binds the default service account in the namespace to the Role:

    ```sh
    sed -e "s/<ROLE_NAME>/${ROLE_NAME}/g" \
      -e "s/<ROLE_BINDING_NAME>/${ROLE_BINDING_NAME}/g" \
      -e "s/<NAMESPACE>/${NAMESPACE}/g" \
      example/rbac/role-binding-template.yaml \
      | kubectl create -f -
    ```

[rbac-templates]: ../../example/rbac/
