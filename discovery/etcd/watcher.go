package etcd

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/kieran091/pilot/discovery"
	"github.com/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type etcdWatcher struct {
	client    *clientv3.Client
	eventChan chan *discovery.Event
	ctx       context.Context
	cancel    context.CancelFunc

	discoveryPath string

	mu sync.RWMutex
}

func NewWatcher(endpoints []string, dialTimeout time.Duration, discoveryPath string) (*etcdWatcher, error) {
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

	w := &etcdWatcher{
		client:        client,
		eventChan:     make(chan *discovery.Event, 100),
		ctx:           ctx,
		cancel:        cancel,
		discoveryPath: discoveryPath,
	}

	return w, nil
}

func (ew *etcdWatcher) Start(_ context.Context) error {
	if err := ew.loadExistingServices(); err != nil {
		return errors.WithMessage(err, "failed to load existing services")
	}

	go ew.watchServices()

	return nil
}

func (ew *etcdWatcher) Stop() error {
	ew.cancel()
	return nil
}

func (ew *etcdWatcher) Watch() <-chan *discovery.Event {
	return ew.eventChan
}

func (ew *etcdWatcher) loadExistingServices() error {
	ctx, cancel := context.WithTimeout(ew.ctx, 10*time.Second)
	defer cancel()

	resp, err := ew.client.Get(ctx, ew.discoveryPath, clientv3.WithPrefix())
	if err != nil {
		return errors.WithMessage(err, "failed to get existing services from etcd")
	}

	servicesMap := make(map[string][]*discovery.Instance)
	metadataMap := make(map[string]*discovery.Definition)

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

		instance := &discovery.Instance{
			ID:   instanceId,
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
				ew.eventChan <- &discovery.Event{
					Type: discovery.Added,
					ServiceInfo: &discovery.Registration{
						Definition: serviceMetadata,
						Instance:   instance,
					},
				}
			}
		}
	}

	return nil
}

func (ew *etcdWatcher) watchServices() {
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

func (ew *etcdWatcher) handleServiceEvent(event *clientv3.Event) {
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
			return
		}

		ew.eventChan <- &discovery.Event{
			Type: discovery.Added,
			ServiceInfo: &discovery.Registration{
				Definition: serviceMetadata,
				Instance: &discovery.Instance{
					ID:   instanceId,
					Addr: serviceMetadata.Addr,
				},
			},
		}
	case clientv3.EventTypeDelete:
		ew.eventChan <- &discovery.Event{
			Type: discovery.Removed,
			ServiceInfo: &discovery.Registration{
				Definition: &discovery.Definition{Name: serviceName},
				Instance: &discovery.Instance{
					ID: instanceId,
				},
			},
		}
	}
}

func (ew *etcdWatcher) parseServicePath(key string) (serviceName string, instanceId string) {
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

func (ew *etcdWatcher) parseServiceMetadata(value []byte) (*discovery.Definition, error) {
	var serviceMetadata discovery.Definition
	if err := sonic.Unmarshal(value, &serviceMetadata); err != nil {
		return nil, errors.WithMessage(err, "failed to unmarshal service metadata")
	}

	return &serviceMetadata, nil
}
