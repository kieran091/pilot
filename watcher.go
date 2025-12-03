package pilot

import "context"

type Watcher interface {
	Start(ctx context.Context) error

	Stop() error

	Watch() <-chan *ServiceEvent

	GetServices() ([]string, error)

	GetService(serviceName string) (*ServiceInfo, error)

	GetInstances(serviceName string) ([]*ServiceInstance, error)
}

type WatcherBuilder interface {
	Build(config any) (Watcher, error)
}
