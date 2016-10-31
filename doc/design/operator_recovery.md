# Operator recovery

- Create TPR
 - If the creation succeed, then the operator is a new one and does not require recovery. END.
- Find all existing clusters
 - loop over the third part resource items to get all created clusters
- Reconstruct clusters
 - for each cluster, find running pods that belong to it by label selection
 - recover the membership by issuing member list call
 - recover nextID by finding the etcd member with the largest ID in its name
