package member

import (
	"time"

	"github.com/GregoryIan/operator/pkg/spec"
	"github.com/juju/errors"
	"github.com/ngaut/log"
	"github.com/pingcap/pd/pd-client"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type MemberType int

const (
	PD MemberType = iota
)

const (
	DefaultDialTimeout    = 5 * time.Second
	DefaultRequestTimeout = 5 * time.Second

	defaultVersion = "rc1"
)

var ServiceAdjustSequence = []MemberType{PD}

func (s MemberType) String() string {
	switch s {
	case PD:
		return "pd"
	}

	panic("can't reach here")
	return ""
}

type MemberSet interface {
	Add(*api.Pod)
	AddOneMember() error
	Diff(other MemberSet) MemberSet
	Members() []*Member
	NeedUpgrade([]*api.Pod) bool
	NotEqualPodSize(int) bool
	PickOne() *Member
	PickOneOldMember(pods []*api.Pod) *Member
	Remove(string)
	Size() int
	SetSpec(*spec.ServiceSpec)
	UpdateMembers(pd.Client) error
}

type Member struct {
	Name    string
	ID      uint64
	Version string
}

type SeedFunc func(*unversioned.Client, string, string, *spec.ClusterSpec) (MemberSet, error)

var seedMapFunc = make(map[MemberType]SeedFunc)

func RegisterSeedMemberFunc(typ MemberType, f SeedFunc) {
	if _, ok := seedMapFunc[typ]; ok {
		log.Fatal("Member: Register called twice %s", typ)
	}
	seedMapFunc[typ] = f
}

// InitSeedMembers initializes tidb cluster, contains only one server for each member type
func InitSeedMembers(kubeCli *unversioned.Client, clusterName, nameSpace string, s *spec.ClusterSpec) (map[MemberType]MemberSet, error) {
	mss := make(map[MemberType]MemberSet)
	for _, m := range ServiceAdjustSequence {
		ms, err := seedMapFunc[m](kubeCli, clusterName, nameSpace, s)
		if err != nil {
			return nil, errors.Trace(err)
		}

		mss[m] = ms
	}

	return mss, nil
}

func GetEmptyMemberSet(tp MemberType) MemberSet {
	switch tp {
	case PD:
		return &PDMemberSet{members: make(map[string]*pdMember)}
	}

	panic("can't reach here")
	return nil
}
