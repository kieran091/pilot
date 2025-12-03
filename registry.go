package pilot

import "context"

type Registry interface {
	Register(ctx context.Context, serviceName, instanceId string, metadata *ServiceMetadata) error

	Deregister(ctx context.Context, serviceName, instanceId string) error

	Update(ctx context.Context, serviceName, instanceId string, metadata *ServiceMetadata) error
}

type RegistryBuilder interface {
	Build(config any) (Registry, error)
}
