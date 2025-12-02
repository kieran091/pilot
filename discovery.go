package pilot

import (
	"context"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/pkg/errors"
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
	Rules             []HTTPRule                    `json:"rules"`
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

type Watcher interface {
	Start(ctx context.Context) error

	Stop() error

	Watch() <-chan *ServiceEvent

	GetServices() ([]string, error)

	GetService(serviceName string) (*ServiceInfo, error)

	GetInstances(serviceName string) ([]*ServiceInstance, error)
}

type Registry interface {
	Register(ctx context.Context, serviceName, instanceId string, metadata *ServiceMetadata) error

	Deregister(ctx context.Context, serviceName, instanceId string) error

	Update(ctx context.Context, serviceName, instanceId string, metadata *ServiceMetadata) error
}

type WatcherBuilder interface {
	Build(config any) (Watcher, error)
}

type RegistryBuilder interface {
	Build(config any) (Registry, error)
}

type Factory struct {
	watcherBuilders  map[Mode]WatcherBuilder
	registryBuilders map[Mode]RegistryBuilder
}

func NewFactory() *Factory {
	factory := &Factory{
		watcherBuilders:  make(map[Mode]WatcherBuilder),
		registryBuilders: make(map[Mode]RegistryBuilder),
	}
	factory.RegisterRegistryBuilder(Etcd, &EtcdRegistryBuilder{})
	factory.RegisterWatcherBuilder(Etcd, &EtcdWatcherBuilder{})

	return factory
}

func (f *Factory) RegisterWatcherBuilder(mode Mode, builder WatcherBuilder) {
	f.watcherBuilders[mode] = builder
}

func (f *Factory) RegisterRegistryBuilder(mode Mode, builder RegistryBuilder) {
	f.registryBuilders[mode] = builder
}

func (f *Factory) CreateWatcher(mode Mode, config any) (Watcher, error) {
	builder, ok := f.watcherBuilders[mode]
	if !ok {
		return nil, errors.Errorf("unsupported discovery mode: %s", mode)
	}
	return builder.Build(config)
}

func (f *Factory) CreateRegistry(mode Mode, config any) (Registry, error) {
	builder, ok := f.registryBuilders[mode]
	if !ok {
		return nil, errors.Errorf("unsupported discovery mode: %s", mode)
	}
	return builder.Build(config)
}
