# Cluster Membership Reconciliation

## Reconciliation

Given a desired size S, we have two membership states:
- running pods P in k8s cluster
- membership M in operator knowledge

Reconciliation is the process to make these two states consistent with the desired size S.

For each reconciling cycle, we get P from k8s API. Comparing M and P, we have the following steps:

1. Remove all pods from set P that does not belong to set M
2. P’ consist of remaining pods of P
3. If P’ = M, the current state matches the membership state. GOTO Resize.
4. If len(P’) < len(M)/2 + 1, quorum lost. Go to recovery process (TODO).
5. Remove one member that in M but does not in P's. GOTO Resize.

### Resize

Given a desired size S, membership M and current running pods P:

1. If len(P) != len(M), then END.
2. If S = len(M), then END.
3. If S > len(M), add one member. END.
4. If S < len(M), remove one member. END
