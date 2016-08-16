# Cluster Membership Reconciliation

## Recovery

Given a desired size S, we have two membership states:
- running pods P in k8s cluster
- membership M in controller knowledge

Recovery is the process to make these two states consistent.  Assuming “len(M) = S” here.

For each reconciling cycle, we get P from k8s API. Comparing M and P, we have the following steps:

1. Remove all pods from set P that does not belong to set M
2. P’ consist of remaining pods of P
3. If P’ = M, the current state matches the membership state. END.
4. If len(P’) < len(M)/2 + 1, quorum lost. Go to recovery process (TODO).
5. Add one missing member. END.

## Resize

(TODO)
