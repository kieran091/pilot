package pilot

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdWatcher struct {
	client    *clientv3.Client
	eventChan chan *ServiceEvent
	ctx       context.Context
	cancel    context.CancelFunc

	discoveryPath string

	mu sync.RWMutex
}

func NewEtcdWatcher(endpoints []string, dialTimeout time.Duration, discoveryPath string) (*EtcdWatcher, error) {
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: dialTimeout,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create etcd client")
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &EtcdWatcher{
		client:        client,
		eventChan:     make(chan *ServiceEvent, 100),
		ctx:           ctx,
		cancel:        cancel,
		discoveryPath: discoveryPath,
	}

	return w, nil
}

func (ew *EtcdWatcher) Start(_ context.Context) error {
	if err := ew.loadExistingServices(); err != nil {
		return errors.WithMessage(err, "failed to load existing services")
	}

	go ew.watchServices()

	return nil
}

func (ew *EtcdWatcher) Stop() error {
	ew.cancel()
	return nil
}

func (ew *EtcdWatcher) Watch() <-chan *ServiceEvent {
	return ew.eventChan
}

func (ew *EtcdWatcher) loadExistingServices() error {
	ctx, cancel := context.WithTimeout(ew.ctx, 10*time.Second)
	defer cancel()

	resp, err := ew.client.Get(ctx, ew.discoveryPath, clientv3.WithPrefix())
	if err != nil {
		return errors.WithMessage(err, "failed to get existing services from etcd")
	}

	servicesMap := make(map[string][]*ServiceInstance)
	metadataMap := make(map[string]*ServiceMetadata)

	for _, kv := range resp.Kvs {
		serviceName, instanceId := ew.parseServicePath(string(kv.Key))
		if serviceName == "" || instanceId == "" {
			continue
		}

		serviceMetadata, err := ew.parseServiceMetadata(kv.Value)
		if err != nil {
			return errors.WithMessagef(err, "failed to parse service metadata for key %s", string(kv.Key))
		}

		if _, exists := metadataMap[serviceName]; !exists {
			metadataMap[serviceName] = serviceMetadata
		}

		instance := &ServiceInstance{
			Id:   instanceId,
			Addr: serviceMetadata.Addr,
		}
		servicesMap[serviceName] = append(servicesMap[serviceName], instance)
	}

	ew.mu.Lock()
	defer ew.mu.Unlock()

	for serviceName, instances := range servicesMap {
		serviceMetadata := metadataMap[serviceName]
		if serviceMetadata != nil {
			for _, instance := range instances {
				ew.eventChan <- &ServiceEvent{
					Type: EventAdd,
					ServiceInfo: &ServiceInfo{
						ServiceMetadata: serviceMetadata,
						Instance:        instance,
					},
				}
			}
		}
	}

	return nil
}

func (ew *EtcdWatcher) watchServices() {
	watchChan := ew.client.Watch(ew.ctx, ew.discoveryPath, clientv3.WithPrefix())

	for {
		select {
		case <-ew.ctx.Done():
			return
		case watchResp := <-watchChan:
			if watchResp.Err() != nil {
				continue
			}
			for _, event := range watchResp.Events {
				ew.handleServiceEvent(event)
			}
		}
	}
}

func (ew *EtcdWatcher) handleServiceEvent(event *clientv3.Event) {
	key := string(event.Kv.Key)
	serviceName, instanceId := ew.parseServicePath(key)
	if serviceName == "" || instanceId == "" {
		return
	}

	ew.mu.Lock()
	defer ew.mu.Unlock()

	switch event.Type {
	case clientv3.EventTypePut:
		serviceMetadata, err := ew.parseServiceMetadata(event.Kv.Value)
		if err != nil {
			defaultLogger.Warn().
				Str("key", string(event.Kv.Value)).
				Err(err).
				Msg("failed to parse service metadata")
			return
		}

		ew.eventChan <- &ServiceEvent{
			Type: EventAdd,
			ServiceInfo: &ServiceInfo{
				ServiceMetadata: serviceMetadata,
				Instance: &ServiceInstance{
					Id:   instanceId,
					Addr: serviceMetadata.Addr,
				},
			},
		}
	case clientv3.EventTypeDelete:
		ew.eventChan <- &ServiceEvent{
			Type: EventDelete,
			ServiceInfo: &ServiceInfo{
				ServiceMetadata: &ServiceMetadata{Name: serviceName},
				Instance: &ServiceInstance{
					Id: instanceId,
				},
			},
		}
	}
}

func (ew *EtcdWatcher) parseServicePath(key string) (serviceName string, instanceId string) {
	if !strings.HasPrefix(key, ew.discoveryPath) {
		return
	}

	key = strings.TrimPrefix(key, ew.discoveryPath)
	key = strings.TrimPrefix(key, "/")
	keyParts := strings.Split(key, "/")
	if len(keyParts) < 2 {
		return
	}

	return keyParts[0], keyParts[1]
}

func (ew *EtcdWatcher) parseServiceMetadata(value []byte) (*ServiceMetadata, error) {
	var serviceMetadata ServiceMetadata
	if err := sonic.Unmarshal(value, &serviceMetadata); err != nil {
		return nil, errors.WithMessage(err, "failed to unmarshal service metadata")
	}

	return &serviceMetadata, nil
}

type EtcdWatcherBuilder struct{}

func (b *EtcdWatcherBuilder) Build(conf any) (Watcher, error) {
	cfg, ok := conf.(*EtcdConfig)
	if !ok {
		return nil, errors.New("invalid config type for EtcdWatcher")
	}

	return NewEtcdWatcher(cfg.Endpoints, cfg.DialTimeout, cfg.DiscoveryPath)
}
