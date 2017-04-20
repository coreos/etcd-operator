package cliente2e

import (
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	// for gcp auth
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kubeconfig string
	namespace  string
	e2eImage   string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	flag.StringVar(&e2eImage, "e2e-image", "", "container image for e2e test")
	flag.StringVar(&namespace, "namespace", "default", "e2e test namespace")
	flag.Parse()
}

func TestClient(t *testing.T) {
	name := "client-e2e-test"
	kubecli := mustNewKubeClient()

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{{
				Name:    name,
				Image:   e2eImage,
				Command: []string{"/bin/sh", "-ec", "cliente2e"},
				Env: []v1.EnvVar{{
					Name:      "MY_POD_NAMESPACE",
					ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
				}},
			}},
		},
	}
	if _, err := kubecli.CoreV1().Pods(namespace).Create(pod); err != nil {
		t.Fatalf("fail to create job (%s): %v", name, err)
	}
	defer kubecli.CoreV1().Pods(namespace).Delete(name, metav1.NewDeleteOptions(1))
	err := retryutil.Retry(5*time.Second, 6, func() (bool, error) {
		pod, err := kubecli.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch pod.Status.Phase {
		case v1.PodSucceeded:
			return true, nil
		case v1.PodFailed:
			return false, errors.New("client e2e failed")
		default:
			t.Logf("status: %v", pod.Status.Phase)
			return false, nil
		}
	})
	if err != nil {
		t.Errorf("fail to finish job: %v", err)
	}
}

func mustNewKubeClient() kubernetes.Interface {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	return kubernetes.NewForConfigOrDie(config)
}
