package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/GregoryIan/operator/pkg/cluster"
	"github.com/GregoryIan/operator/pkg/spec"
	"github.com/GregoryIan/operator/pkg/util"
	"github.com/juju/errors"
	"github.com/ngaut/log"
	k8sapi "k8s.io/kubernetes/pkg/api"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

const (
	tprName = "tidb-cluster.pingcap.com"
)

var (
	// ErrVersionOutdated represents the error whose requested version is outdated in apiserver
	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")
	initRetryWaitTime  = 30 * time.Second
)

// raw event struct from k8s' api-server
type rawEvent struct {
	Type   string
	Object json.RawMessage
}

// Event describes the new modification spec
type Event struct {
	Type   string
	Object *spec.TiDBCluster
}

type Controller struct {
	MasterHost string
	Namespace  string
	KubeCli    *unversioned.Client

	clusters    map[string]*cluster.Cluster
	stopChMap   map[string]chan struct{}
	waitCluster sync.WaitGroup
}

func New(masterHost string, nameSpace string, kubeCli *unversioned.Client) *Controller {
	return &Controller{
		MasterHost: masterHost,
		Namespace:  nameSpace,
		KubeCli:    kubeCli,

		clusters:  make(map[string]*cluster.Cluster),
		stopChMap: map[string]chan struct{}{},
	}
}

func (c *Controller) Run() error {
	var (
		watchVersion string
		err          error
	)

	for {
		watchVersion, err = c.initResource()
		if err == nil {
			break
		}
		log.Errorf("initialization failed: %v", err)
		log.Infof("retry in %v...", initRetryWaitTime)
		time.Sleep(initRetryWaitTime)
	}

	log.Infof("starts running from watch version: %s", watchVersion)

	defer func() {
		for _, stopC := range c.stopChMap {
			close(stopC)
		}
		c.waitCluster.Wait()
	}()

	eventCh, errCh := c.monitor(watchVersion)

	go func() {
		for event := range eventCh {
			clusterName := event.Object.ObjectMeta.Name
			switch event.Type {
			case "ADDED":
				log.Infof("ADDED clustername %s, spec %+v", clusterName, *event.Object.Spec.PD)
				stopC := make(chan struct{})
				nc, err := cluster.New(c.makeClusterConfig(clusterName), &event.Object.Spec, stopC, &c.waitCluster)
				if err != nil {
					log.Errorf("cluster (%q) is dead: %v", clusterName, err)
					continue
				}

				c.stopChMap[clusterName] = stopC
				c.clusters[clusterName] = nc
			case "MODIFIED":
				log.Infof("MODIFIED clustername %s, spec %+v", clusterName, event.Object.Spec)
				if c.clusters[clusterName] == nil {
					log.Warningf("ignore modification: cluster %q not found (or dead)", clusterName)
					break
				}
				c.clusters[clusterName].Update(&event.Object.Spec)
			case "DELETED":
				log.Infof("DELETED clustername %s, spec %+v", clusterName, event.Object.Spec)
				if c.clusters[clusterName] == nil {
					log.Warningf("ignore deletion: cluster %q not found (or dead)", clusterName)
					break
				}
				c.clusters[clusterName].Delete()
				delete(c.clusters, clusterName)
			}
		}
	}()
	return <-errCh
}

type TiDBClusterList struct {
	unversionedAPI.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	unversionedAPI.ListMeta `json:"metadata,omitempty"`
	// Items is a list of third party objects
	Items []spec.TiDBCluster `json:"items"`
}

func (c *Controller) findAllClusters() (string, error) {
	log.Info("finding existing clusters...")
	resp, err := util.ListTiDBCluster(c.MasterHost, c.Namespace, c.KubeCli.RESTClient.Client)
	if err != nil {
		return "", err
	}
	d := json.NewDecoder(resp.Body)
	list := &TiDBClusterList{}
	if err := d.Decode(list); err != nil {
		return "", err
	}
	for _, item := range list.Items {
		clusterName := item.Name
		stopC := make(chan struct{})
		nc, err := cluster.Restore(c.makeClusterConfig(clusterName), &item.Spec, stopC, &c.waitCluster)
		if err != nil {
			log.Errorf("cluster (%q) is dead: %v", clusterName, err)
			continue
		}
		c.stopChMap[clusterName] = stopC
		c.clusters[clusterName] = nc
	}
	return list.ListMeta.ResourceVersion, nil
}

func (c *Controller) makeClusterConfig(clusterName string) cluster.Config {
	return cluster.Config{
		Name:      clusterName,
		Namespace: c.Namespace,
		KubeCli:   c.KubeCli,
	}
}

func (c *Controller) initResource() (string, error) {
	watchVersion := "0"
	err := c.createTPR()
	if err != nil {
		if util.IsKubernetesResourceAlreadyExistError(err) {
			// TPR has been initialized before. We need to recover existing cluster.
			watchVersion, err = c.findAllClusters()
			if err != nil {
				return "", err
			}
		} else {
			return "", errors.Errorf("fail to create TPR: %v", err)
		}
	}
	return watchVersion, nil
}

func (c *Controller) createTPR() error {
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: k8sapi.ObjectMeta{
			Name: tprName,
		},
		Versions: []extensions.APIVersion{
			{Name: "v1"},
		},
		Description: "Managed tidb clusters",
	}
	_, err := c.KubeCli.ThirdPartyResources().Create(tpr)
	if err != nil {
		return err
	}

	return util.WaitTiDBTPRReady(c.KubeCli.Client, 3*time.Second, 30*time.Second, c.MasterHost, c.Namespace)
}

func (c *Controller) monitor(watchVersion string) (<-chan *Event, <-chan error) {
	host := c.MasterHost
	ns := c.Namespace
	httpClient := c.KubeCli.Client

	eventCh := make(chan *Event)
	// On unexpected error case, controller should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			resp, err := util.WatchTiDBCluster(host, ns, httpClient, watchVersion)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				errCh <- errors.New("Invalid status code: " + resp.Status)
				return
			}

			log.Infof("start watching at %v", watchVersion)
			decoder := json.NewDecoder(resp.Body)
			for {
				ev, st, err := pollEvent(decoder) // streaming way
				if err != nil {
					if err == io.EOF {
						log.Debug("apiserver closed stream")
						break
					}
					log.Errorf("received invalid event from API server: %v", err)
					errCh <- err
					return
				}

				if st != nil {
					if st.Code == http.StatusGone {
						errCh <- ErrVersionOutdated
						return
					}
					log.Fatalf("unexpected status response from API server: %v", st.Message)
				}
				log.Debugf("etcd cluster event: %v %v", ev.Type, ev.Object.Spec)

				watchVersion = ev.Object.ObjectMeta.ResourceVersion
				eventCh <- ev
			}

			resp.Body.Close()
		}
	}()

	return eventCh, errCh
}

func pollEvent(decoder *json.Decoder) (*Event, *unversionedAPI.Status, error) {
	re := &rawEvent{}
	err := decoder.Decode(re)
	if err != nil {
		if err == io.EOF {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("fail to decode raw event from apiserver (%v)", err)
	}

	if re.Type == "ERROR" {
		status := &unversionedAPI.Status{}
		err = json.Unmarshal(re.Object, status)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode (%s) into unversioned.Status (%v)", re.Object, err)
		}
		return nil, status, nil
	}

	ev := &Event{
		Type:   re.Type,
		Object: &spec.TiDBCluster{},
	}
	err = json.Unmarshal(re.Object, ev.Object)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to unmarshal EtcdCluster object from data (%s): %v", re.Object, err)
	}
	return ev, nil, nil
}
