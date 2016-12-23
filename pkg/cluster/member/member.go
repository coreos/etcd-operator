package member

import (
	"fmt"
)

type MemberType int

const (
	PD MemberType = iota
	TIKV
	TIDB
)

var EnumTypeName = []string{
	PD:   "pd",
	TIKV: "tikv",
	TIDB: "tidb",
}

type SeedFunc func(string) MemberSet

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

func SeedMemberSet(typ MemberType, firstMemberName string) (MemberSet, error) {
	seedFunc, ok := seedMapF[typ]
	if !ok {
		return fmt.Errorf("no such memberset type")
	}
	ms := seedFunc(firstMemberName)
	return ms, nil
}
