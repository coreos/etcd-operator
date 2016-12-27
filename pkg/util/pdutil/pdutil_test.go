package pdutil

import (
	"testing"

	"github.com/juju/errors"
	"github.com/ngaut/log"
	. "github.com/pingcap/check"
)

var _ = Suite(&testPDSuite{})

var (
	errTimeout = errors.New("timeout")
)

type testPDSuite struct {
}

func TestSuite(t *testing.T) {
	TestingT(t)
}

func (t *testPDSuite) TestList(c *C) {
	cli := New([]string{"http://127.0.0.1:2379"})
	res, err := cli.PDMemberList()
	c.Assert(err, IsNil)
	log.Debugf("%+v", res)
	r, err := cli.KVMemberList()
	c.Assert(err, IsNil)
	log.Debugf("%+v", r)
}
