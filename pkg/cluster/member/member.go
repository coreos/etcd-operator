package member

import (
	"fmt"

	"github.com/GregoryIan/oprator/spec"
)

type MemberType int

const (
	PD MemberType = iota
	TiKV
	TiDB
)

var StartUpSequence = []MemberType{PD, TiKV, TiDB}

func (s MemberType) String() string {
	switch s {
	case TiDB:
		return "tidb"
	case TiKV:
		return "tikv"
	case PD:
		return "pd"
	}
}

type SeedFunc func(*unversioned.Client, string, string, *spec.ClusterSpec) MemberSet

var seedMapFunc = make(map[MemberType]SeedFunc)

type MemberSet interface {
	Diff(other MemberSet) MemberSet
	Size() int
	Type() MemberType
}

func RegisterSeedMemberFunc(typ MemberType, f SeedFunc) {
	if _, ok := seedMapFunc[typ]; ok {
		return fmt.Errorf("already exists")
	}
	seedMapFunc[typ] = f
}

func InitSeedMembers() {
	return seedMapFunc
}
