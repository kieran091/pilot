package pilot

import (
	"context"
	"fmt"
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

	servicesMap map[string]*ServiceInfo
	mu          sync.RWMutex
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
		servicesMap:   make(map[string]*ServiceInfo),
	}

	return w, nil
}

func (ew *EtcdWatcher) Start(ctx context.Context) error {
	if err := ew.loadExistingServices(); err != nil {
		return errors.WithMessage(err, "failed to load existing services")
	}

	// 监听服务变更
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

func (ew *EtcdWatcher) GetServices() ([]string, error) {
	ew.mu.Lock()
	defer ew.mu.Unlock()

	services := make([]string, 0, len(ew.servicesMap))
	for serviceName := range ew.servicesMap {
		services = append(services, serviceName)
	}

	return services, nil
}

func (ew *EtcdWatcher) GetService(serviceName string) (*ServiceInfo, error) {
	ew.mu.RLock()
	defer ew.mu.RUnlock()

	service, exists := ew.servicesMap[serviceName]
	if !exists {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	return service, nil
}

func (ew *EtcdWatcher) GetInstances(serviceName string) ([]*ServiceInstance, error) {
	service, err := ew.GetService(serviceName)
	if err != nil {
		return nil, err
	}

	return service.Instances, nil
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
		metadata := metadataMap[serviceName]
		if metadata == nil {
			ew.servicesMap[serviceName] = &ServiceInfo{
				ServiceMetadata: metadata,
				Instances:       instances,
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
		metadata, err := ew.parseServiceMetadata(event.Kv.Value)
		if err != nil {
			return
		}

		instance := &ServiceInstance{
			Id:   instanceId,
			Addr: metadata.Addr,
		}

		serviceInfo, exists := ew.servicesMap[serviceName]
		if !exists {
			serviceInfo = &ServiceInfo{
				ServiceMetadata: metadata,
				Instances:       []*ServiceInstance{instance},
			}
			ew.servicesMap[serviceName] = serviceInfo
			ew.eventChan <- &ServiceEvent{
				Type:    EventAdd,
				Service: serviceInfo,
			}
		} else {
			found := false
			for i, inst := range serviceInfo.Instances {
				if inst.Id == instance.Id {
					serviceInfo.Instances[i] = instance
					found = true
					break
				}
			}

			if !found {
				serviceInfo.Instances = append(serviceInfo.Instances, instance)
			}

			serviceInfo.ServiceMetadata = metadata
			ew.eventChan <- &ServiceEvent{
				Type:    EventUpdate,
				Service: serviceInfo,
			}
		}
	case clientv3.EventTypeDelete:
		serviceInfo, exists := ew.servicesMap[serviceName]
		if !exists {
			return
		}

		found := false
		newInstances := make([]*ServiceInstance, 0, len(serviceInfo.Instances))
		for _, inst := range serviceInfo.Instances {
			if inst.Id == instanceId {
				found = true
				continue
			}
			newInstances = append(newInstances, inst)
		}

		if !found {
			return
		}

		if len(newInstances) == 0 {
			delete(ew.servicesMap, serviceName)
			ew.eventChan <- &ServiceEvent{
				Type:    EventDelete,
				Service: serviceInfo,
			}
		} else {
			serviceInfo.Instances = newInstances
			ew.eventChan <- &ServiceEvent{
				Type:    EventUpdate,
				Service: serviceInfo,
			}
		}
	}
}

func (ew *EtcdWatcher) parseServicePath(key string) (serviceName string, instanceId string) {
	if !strings.HasPrefix(key, ew.discoveryPath) {
		return
	}

	// 移除前缀并拆分为多段
	keyParts := strings.Split(strings.TrimPrefix(key, ew.discoveryPath), "/")
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
