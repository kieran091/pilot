package pilot

import "context"

type Watcher interface {
	Start(ctx context.Context) error

	Stop() error

	Watch() <-chan *ServiceEvent
}

type WatcherBuilder interface {
	Build(config any) (Watcher, error)
}
