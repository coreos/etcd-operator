package etcdutil

import (
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"golang.org/x/net/context"
)

func WaitMemberReady(cli *clientv3.Client) error {
	for {
		_, err := cli.Get(context.TODO(), "/")
		if err == rpctypes.ErrNotCapable {
			continue
		}
		return err
	}
}
