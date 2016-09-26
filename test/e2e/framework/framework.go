package framework

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
)

var Global *Framework

type Framework struct {
	KubeClient *unversioned.Client
	MasterHost string
	Namespace  *api.Namespace
}

func New(kubeconfig string, baseName string) (*Framework, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	cli, err := unversioned.New(config)
	if err != nil {
		return nil, err
	}
	namespace, err := cli.Namespaces().Create(&api.Namespace{
		ObjectMeta: api.ObjectMeta{
			GenerateName: fmt.Sprintf("e2e-test-%v-", baseName),
		},
	})
	if err != nil {
		return nil, err
	}

	return &Framework{
		MasterHost: config.Host,
		KubeClient: cli,
		Namespace:  namespace,
	}, nil
}

func (f *Framework) Setup(ctrlImage string) error {
	if err := f.setupEtcdController(ctrlImage); err != nil {
		logrus.Errorf("fail to setup etcd controller: %v", err)
		return err
	}
	logrus.Info("e2e setup successfully")
	return nil
}

func (f *Framework) Teardown() error {
	// TODO: delete TPR
	if err := f.KubeClient.Namespaces().Delete(f.Namespace.Name); err != nil {
		return err
	}
	logrus.Info("e2e teardown successfully")
	return nil
}

func (f *Framework) setupEtcdController(ctrlImage string) error {
	// TODO: unify this and the yaml file in example/
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name:   "kube-etcd-controller",
			Labels: map[string]string{"name": "kube-etcd-controller"},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Name:  "kube-etcd-controller",
					Image: ctrlImage,
					Env: []api.EnvVar{
						{
							Name:      "MY_POD_NAMESPACE",
							ValueFrom: &api.EnvVarSource{FieldRef: &api.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
						},
					},
				},
			},
		},
	}

	_, err := f.KubeClient.Pods(f.Namespace.Name).Create(pod)
	if err != nil {
		return err
	}
	err = k8sutil.WaitEtcdTPRReady(f.KubeClient.Client, 5*time.Second, 90*time.Second, f.MasterHost, f.Namespace.Name)
	if err != nil {
		return err
	}
	logrus.Info("etcd controller created successfully")
	return nil
}
