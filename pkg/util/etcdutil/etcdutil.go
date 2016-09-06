package etcdutil

import (
	"fmt"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"golang.org/x/net/context"
)

func WaitMemberReady(cli *clientv3.Client, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting etcd cluster ready")
		default:
		}
		_, err := cli.Get(context.TODO(), "/")
		if err == rpctypes.ErrNotCapable {
			time.Sleep(1 * time.Second)
			continue
		}
		timer.Stop()
		return err
	}
}
