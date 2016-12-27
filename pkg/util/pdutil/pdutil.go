package pdutil

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/ngaut/log"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/pingcap/kvproto/pkg/pdpb"
)

const retryCount = 20

var (
	memberPrefix = "pd/api/v1/members"
	storesPrefix = "pd/api/v1/stores"
	storePrefix  = "pd/api/v1/store"
)

type StoreInfo struct {
	Store *metapb.Store `json:"store"`
}

type StoresInfo struct {
	Count  int         `json:"count"`
	Stores []StoreInfo `json:"stores"`
}

type Client struct {
	url []string
	cli *http.Client
}

func New(u []string) *Client {
	c := &Client{
		cli: &http.Client{},
		url: u,
	}
	return c
}

func readJSON(r io.ReadCloser, data interface{}) error {
	defer r.Close()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Trace(err)
	}

	err = json.Unmarshal(b, data)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func parseUrl(addr string) (*url.URL, error) {
	u, err := url.Parse(addr)
	if err != nil {
		log.Errorf("addr:%s err:%+v", addr, err)
		return nil, err
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	return u, nil
}

//TODO add context with timeout
func (c *Client) PDMemberList() ([]*pdpb.PDMember, error) {
	for i := 0; i < retryCount; i++ {
		for _, addr := range c.url {
			log.Info("start get pd member", addr)
			u, err := parseUrl(addr)
			if err != nil {
				continue
			}
			apiUrl := fmt.Sprintf("%s/%s", u, memberPrefix)
			r, err := c.cli.Get(apiUrl)
			if err != nil {
				log.Info(err)
				continue
			}
			defer r.Body.Close()
			if r.StatusCode != http.StatusOK {
				io.Copy(os.Stdout, r.Body)
				continue
			}
			respMembers := make(map[string][]*pdpb.PDMember)
			err = readJSON(r.Body, &respMembers)
			if err != nil {
				log.Debugf("readJSON: %s", err)
				continue
			}
			if res, ok := respMembers["members"]; ok {
				return res, nil
			}
		}
		time.Sleep(time.Second)
	}
	return nil, errors.Errorf("cann't get pd members")
}

func (c *Client) PDMemberRemove(name string) error {
	for i := 0; i < retryCount; i++ {
		for _, addr := range c.url {
			log.Info("Start delete pd member", addr)
			u, err := parseUrl(addr)
			if err != nil {
				continue
			}
			apiUrl := fmt.Sprintf("%s/%s/%s", u, memberPrefix, name)
			r, _ := http.NewRequest("DELETE", apiUrl, nil)
			reps, err := c.cli.Do(r)
			if err != nil {
				log.Infof("PD delete error:%s", err)
				continue
			}
			defer reps.Body.Close()
			if reps.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.Errorf("cann't remove pd member %s", name)
}

//TODO add context with timeout
func (c *Client) KVMemberList() (*StoresInfo, error) {
	for i := 0; i < retryCount; i++ {
		for _, addr := range c.url {
			log.Info("start get tikv stores", addr)
			u, err := parseUrl(addr)
			if err != nil {
				continue
			}

			apiUrl := fmt.Sprintf("%s/%s", u, storesPrefix)
			r, err := c.cli.Get(apiUrl)
			if err != nil {
				log.Info(err)
				continue
			}
			defer r.Body.Close()
			if r.StatusCode != http.StatusOK {
				io.Copy(os.Stdout, r.Body)
				continue
			}
			res := &StoresInfo{}
			err = readJSON(r.Body, res)
			if err != nil {
				log.Debugf("readJSON: %s", err)
				continue
			}
			return res, nil
		}
		time.Sleep(time.Second)
	}
	return nil, errors.Errorf("cann't get pd members")
}

func (c *Client) KVMemberRemove(id int) error {
	for i := 0; i < retryCount; i++ {
		for _, addr := range c.url {
			log.Info("Start delete stores", addr)
			u, err := parseUrl(addr)
			if err != nil {
				continue
			}
			apiUrl := fmt.Sprintf("%s/%s/%d", u, storePrefix, id)
			r, _ := http.NewRequest("DELETE", apiUrl, nil)
			reps, err := c.cli.Do(r)
			if err != nil {
				log.Infof("Store %d delete error:%s", id, err)
				continue
			}
			defer reps.Body.Close()
			if reps.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.Errorf("cann't remove  tore %d", id)
}
