package main

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/coreos/etcd-operator/pkg/etcd-backup-operator/controller"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	version "github.com/coreos/etcd-operator/version/etcd-backup-operator"

	"github.com/Sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

func main() {
	namespace := os.Getenv("MY_POD_NAMESPACE")
	if len(namespace) == 0 {
		logrus.Fatalf("must set env MY_POD_NAMESPACE")
	}
	name := os.Getenv("MY_POD_NAME")
	if len(name) == 0 {
		logrus.Fatalf("must set env MY_POD_NAME")
	}
	id, err := os.Hostname()
	if err != nil {
		logrus.Fatalf("failed to get hostname: %v", err)
	}

	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("etcd-backup-operator Version: %v", version.Version)
	logrus.Infof("Git SHA: %s", version.GitSHA)

	kubecli := k8sutil.MustNewKubeClient()
	rl, err := resourcelock.New(
		resourcelock.EndpointsResourceLock,
		namespace,
		"etcd-backup-operator",
		kubecli.Core(),
		resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: createRecorder(kubecli, name, namespace),
		},
	)
	if err != nil {
		logrus.Fatalf("error creating lock: %v", err)
	}

	leaderelection.RunOrDie(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				logrus.Fatalf("leader election lost")
			},
		},
	})
}

func createRecorder(kubecli kubernetes.Interface, name, namespace string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubecli.Core().RESTClient()).Events(namespace)})
	return eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: name})
}

func run(stop <-chan struct{}) {
	c := controller.New()
	err := c.Start(context.TODO())
	if err != nil {
		logrus.Infof("operator stopped with %v", err)
	}
}
