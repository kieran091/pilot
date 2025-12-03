package pilot

import "github.com/pkg/errors"

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
