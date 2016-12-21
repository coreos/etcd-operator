# Roadmap

This document defines a high level roadmap for the etcd cluster operator development.

The dates below should not be considered authoritative, but rather indicative of the projected timeline of the project.


### 2017 Q1

#### Features

- Backup and recovery
  - Recover an etcd cluster from a PV backup -> finished
  - Backup an etcd cluster to S3 -> finished
  - Recover an etcd cluster from a S3 backup -> finished

- Operationality
  - Pause/Resume the control for an etcd cluster -> finished

- Metrics and logging
  - More structured logging
      - Add prefix for different clusters -> finished
      - Use infof, warningf, errorf consistently -> finished
  - Expose operator metrics
      - How many clusters it manages
      - How many actions it does
   - Expose the running status of the etcd cluster
      - cluster size, version
   - Expose errors 
     -  bad version, bad cluster size, dead cluster

- Security
  - Server side TLS support


#### Stability/Reliability

- Verify user inputs
  - desired version
  - cluster size (do not stupidly increase cluster size to an unreasonable number)

- More soak testing
- 60%+ unit tests coverage
