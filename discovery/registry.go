package discovery

import (
	"context"
)

type Registry interface {
	Register(ctx context.Context, serviceName, instanceId string, definition *Definition) error

	Deregister(ctx context.Context, serviceName, instanceId string) error

	Update(ctx context.Context, serviceName, instanceId string, definition *Definition) error
}