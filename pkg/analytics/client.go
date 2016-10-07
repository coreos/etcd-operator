package analytics

import (
	"sync"

	ga "github.com/jpillora/go-ogle-analytics"
)

const (
	id       = "UA-42684979-8"
	category = "etcd-controller"
)

var (
	mu     sync.Mutex
	client *ga.Client
)

func Enable() {
	mu.Lock()
	defer mu.Unlock()
	client = mustNewClient()
}

func Disable() {
	mu.Lock()
	defer mu.Unlock()
	client = nil
}

func send(e *ga.Event) {
	mu.Lock()
	c := client
	mu.Unlock()

	if c == nil {
		return
	}
	// error is ignored intentionally. we try to send event to GA in a best effort approach.
	c.Send(e)
}

func mustNewClient() *ga.Client {
	client, err := ga.NewClient("UA-42684979-8")
	if err != nil {
		panic(err)
	}
	return client
}

func ClusterCreated() {
	send(ga.NewEvent(category, "cluster_created"))
}

func ClusterDeleted() {
	send(ga.NewEvent(category, "cluster_deleted"))
}
