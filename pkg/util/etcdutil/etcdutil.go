package etcdutil

import (
	"fmt"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/coreos/kube-etcd-controller/pkg/util/constants"
	"golang.org/x/net/context"
)

func WaitMemberReady(cli *clientv3.Client, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting etcd cluster ready")
		default:
		}
		ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		_, err := cli.Get(ctx, "/")
		if err == rpctypes.ErrNotCapable {
			time.Sleep(1 * time.Second)
			continue
		}
		return err
	}
}
