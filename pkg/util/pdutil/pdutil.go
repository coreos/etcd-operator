package pdutil

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/ngaut/log"
	"github.com/pingcap/kvproto/pkg/pdpb"
)

var (
	memberPrefix = "pd/api/v1/members"
)

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

//TODO add context with timeout
func (c *Client) PDMemberList() ([]*pdpb.PDMember, error) {
	for _, addr := range c.url {
		u, err := url.Parse(addr)
		if err != nil {
			continue
		}
		if u.Scheme == "" {
			u.Scheme = "http"
		}

		apiUrl := fmt.Sprint("%s/%s", u, memberPrefix)
		r, err := c.cli.Get(apiUrl)
		if err != nil {
			log.Info(err)
			continue
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			continue
		}
		respMembers := make(map[string][]*pdpb.PDMember)
		readJSON(r.Body, respMembers)
		if res, ok := respMembers["members"]; ok {
			return res, nil
		}
	}
	return nil, errors.Errorf("cann't get pd members")
}
