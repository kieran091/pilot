package discovery

import (
	"context"
)

type Watcher interface {
	Start(ctx context.Context) error

	Stop() error

	Watch() <-chan *Event
}