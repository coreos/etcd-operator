package k8sutil

import (
	"testing"
	"fmt"
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
)

func TestDefaultBusyboxImageName(t *testing.T) {
	policy := &api.PodPolicy{}
	image:=ImageNameBusybox(policy)
	expected:= fmt.Sprintf("%s:%v",defaultBusyboxRepository , defaultBusyboxVersion)
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}

func TestDefaultNilBusyboxImageName(t *testing.T) {
	image:=ImageNameBusybox(nil)
	expected:= fmt.Sprintf("%s:%v",defaultBusyboxRepository , defaultBusyboxVersion)
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}

func TestSetBusyboxImageName(t *testing.T) {
	policy := &api.PodPolicy{
		BusyboxVersion:"1.3.2",
		BusyboxRepository:"myRepo/busybox",
	}
	image:=ImageNameBusybox(policy)
	expected:= fmt.Sprintf("%s:%v","myRepo/busybox" , "1.3.2")
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}