package e2e

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"testing"

	"github.com/coreos/etcd/embed"
	"github.com/coreos/kube-etcd-controller/pkg/spec"
	"github.com/coreos/kube-etcd-controller/test/e2e/framework"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

func TestCreateClusterWithSeedMember(t *testing.T) {
	dir := path.Join(os.TempDir(), fmt.Sprintf("embed-etcd"))
	os.RemoveAll(dir)
	defer os.RemoveAll(dir)

	embedCfg := embed.NewConfig()
	embedCfg.Dir = dir
	lpurl, _ := url.Parse("http://0.0.0.0:2380")
	lcurl, _ := url.Parse("http://0.0.0.0:2379")
	embedCfg.LCUrls = []url.URL{*lcurl}
	embedCfg.LPUrls = []url.URL{*lpurl}

	e, err := embed.StartEtcd(embedCfg)
	if err != nil {
		t.Fatal(err)
	}

	<-e.Server.ReadyNotify()
	fmt.Println("etcdserver is ready")

	f := framework.Global

	c := &spec.EtcdCluster{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "EtcdCluster",
			APIVersion: "coreos.com/v1",
		},
		ObjectMeta: api.ObjectMeta{
			GenerateName: "etcd-test-seed-",
		},
		Spec: spec.ClusterSpec{
			Size: 3,
			Seed: &spec.SeedPolicy{
				MemberClientEndpoints: []string{embedCfg.ACUrls[0].String()},
				RemoveDelay:           30,
			},
		},
	}

	testEtcd, err := createEtcdCluster(f, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 120); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}
