package framework

import (
	"flag"
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

// Setup setups a test framework and points "Global" to it.
func Setup() error {
	kubeconfig := flag.String("kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	ctrlImage := flag.String("controller-image", "", "controller image, e.g. gcr.io/coreos-k8s-scale-testing/kube-etcd-controller")
	ns := flag.String("namespace", "default", "e2e test namespace")
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return err
	}
	cli, err := unversioned.New(config)
	if err != nil {
		return err
	}
	var namespace *api.Namespace
	if *ns != "default" {
		namespace, err = cli.Namespaces().Create(&api.Namespace{
			ObjectMeta: api.ObjectMeta{
				Name: *ns,
			},
		})
	} else {
		namespace, err = cli.Namespaces().Get("default")
	}
	if err != nil {
		return err
	}

	Global = &Framework{
		MasterHost: config.Host,
		KubeClient: cli,
		Namespace:  namespace,
	}
	return Global.setup(*ctrlImage)
}

func Teardown() error {
	if Global.Namespace.Name != "default" {
		if err := Global.KubeClient.Namespaces().Delete(Global.Namespace.Name); err != nil {
			return err
		}
	}
	// TODO: delete TPR
	// TODO: check all deleted and wait
	Global = nil
	logrus.Info("e2e teardown successfully")
	return nil
}

func (f *Framework) setup(ctrlImage string) error {
	if err := f.setupEtcdController(ctrlImage); err != nil {
		logrus.Errorf("fail to setup etcd controller: %v", err)
		return err
	}
	logrus.Info("e2e setup successfully")
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
			RestartPolicy: api.RestartPolicyNever,
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
