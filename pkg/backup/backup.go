package backup

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/kube-etcd-controller/pkg/util/etcdutil"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type Backup struct {
	kclient *unversioned.Client

	clusterName string
	policy      Policy
	listenAddr  string
	backupDir   string

	backupNow chan chan struct{}
}

func New(kclient *unversioned.Client, clusterName string, policy Policy, listenAddr string) *Backup {
	return &Backup{
		kclient:     kclient,
		clusterName: clusterName,
		policy:      policy,
		listenAddr:  listenAddr,
		backupDir:   "/home/backup/",

		backupNow: make(chan chan struct{}),
	}
}

func (b *Backup) Run() {
	// It will be no-op if backup dir existed.
	if err := os.MkdirAll(b.backupDir, 0700); err != nil {
		panic(err)
	}

	go b.startHTTP()

	lastSnapRev := int64(0)
	interval := defaultSnapshotInterval
	if b.policy.SnapshotIntervalInSecond != 0 {
		interval = time.Duration(b.policy.SnapshotIntervalInSecond) * time.Second
	}
	for {
		// todo: make ackchan a chan of error
		// so we can propogate error to the http handler
		var ackchan chan struct{}
		select {
		case <-time.After(interval):
		case ackchan = <-b.backupNow:
			logrus.Info("received a backup request")
		}
		pods, err := b.kclient.Pods("default").List(api.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app":          "etcd",
				"etcd_cluster": b.clusterName,
			}),
		})
		if err != nil {
			panic(err)
		}
		if len(pods.Items) == 0 {
			logrus.Warning("no running pods found.")
			continue
		}
		member, rev, err := getMemberWithMaxRev(pods)
		if err != nil {
			logrus.Error(err)
			continue
		}
		if member == nil {
			logrus.Warning("no reachable member")
			continue
		}
		if rev == lastSnapRev {
			logrus.Info("skipped creating new backup: no change since last time")
			continue
		}

		log.Printf("saving backup for cluster (%s)", b.clusterName)
		if err := writeSnap(member, b.backupDir, rev); err != nil {
			logrus.Errorf("write snapshot failed: %v", err)
			continue
		}
		lastSnapRev = rev

		if ackchan != nil {
			close(ackchan)
		}
	}
}

func writeSnap(m *etcdutil.Member, backupDir string, rev int64) error {
	cfg := clientv3.Config{
		Endpoints:   []string{m.ClientAddr()},
		DialTimeout: 5 * time.Second,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create etcd client (%v)", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	rc, err := etcdcli.Maintenance.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to receive snapshot (%v)", err)
	}
	defer rc.Close()

	// TODO: custom backup dir
	tmpfile, err := ioutil.TempFile(backupDir, "snapshot")
	n, err := io.Copy(tmpfile, rc)
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		return fmt.Errorf("failed to save snapshot: %v", err)
	}
	cancel()
	tmpfile.Close()
	nextSnapshotName := path.Join(backupDir, makeFilename(rev))
	err = os.Rename(tmpfile.Name(), nextSnapshotName)
	if err != nil {
		os.Remove(tmpfile.Name())
		return fmt.Errorf("rename snapshot from %s to %s failed: %v", tmpfile.Name(), nextSnapshotName, err)
	}
	log.Printf("saved snapshot %s (size: %d) successfully", nextSnapshotName, n)
	return nil
}

func makeFilename(rev int64) string {
	return fmt.Sprintf("%016x.backup", rev)
}

func getMemberWithMaxRev(pods *api.PodList) (*etcdutil.Member, int64, error) {
	var member *etcdutil.Member
	maxRev := int64(0)
	for i := range pods.Items {
		m := &etcdutil.Member{Name: pods.Items[i].Name}
		cfg := clientv3.Config{
			Endpoints:   []string{m.ClientAddr()},
			DialTimeout: 5 * time.Second,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create etcd client (%v)", err)
		}
		resp, err := etcdcli.Get(context.TODO(), "/", clientv3.WithSerializable())
		if err != nil {
			return nil, 0, fmt.Errorf("etcdcli.Get failed: %v", err)
		}
		logrus.Infof("member: %s, revision: %d", m.Name, resp.Header.Revision)
		if resp.Header.Revision > maxRev {
			maxRev = resp.Header.Revision
			member = m
		}
	}
	return member, maxRev, nil
}
