package pilot

import (
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

type Mode string

const (
	Etcd Mode = "etcd"
)

type ServiceInstance struct {
	Id   string
	Addr string
}

type ServiceMetadata struct {
	Name              string                        `json:"name"`
	Addr              string                        `json:"addr"`
	Rules             []Rule                        `json:"rules"`
	ProtoDSL          string                        `json:"proto_dsl"`
	FileDescriptorSet *descriptor.FileDescriptorSet `json:"-"`
}

type ServiceInfo struct {
	*ServiceMetadata
	Instances []*ServiceInstance
}

type ServiceEvent struct {
	Type        EventType
	ServiceInfo *ServiceInfo
}

type EventType int

const (
	EventAdd EventType = iota
	EventDelete
	EventUpdate
)
