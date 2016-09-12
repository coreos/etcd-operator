package framework

import (
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
)

type Framework struct {
	KubeClient *unversioned.Client
	MasterHost string
}

func New(kubeconfig string) (*Framework, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	cli, err := unversioned.New(config)
	if err != nil {
		return nil, err
	}

	return &Framework{
		MasterHost: config.Host,
		KubeClient: cli,
	}, nil
}
