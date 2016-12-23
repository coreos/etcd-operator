package cluster

type clusterEvent struct {
	typ clusterEventType
	spc spcc.ClusterSpec
}
