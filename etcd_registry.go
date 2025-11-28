package pilot

import (
	"context"
	"path"
	"time"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

// EtcdRegistry etcd适配器实现
type EtcdRegistry struct {
	client        *clientv3.Client
	leaseId       clientv3.LeaseID
	keepAliveChan <-chan *clientv3.LeaseKeepAliveResponse
	discoveryPath string
}

func NewEtcdRegistry(endpoints []string, dialTimeout time.Duration, discoveryPath string) (*EtcdRegistry, error) {
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

	return &EtcdRegistry{
		client:        client,
		discoveryPath: discoveryPath,
	}, nil
}

func (er *EtcdRegistry) Register(ctx context.Context, serviceName, instanceId string, metadata *ServiceMetadata) error {
	key := path.Join(er.discoveryPath, serviceName, instanceId)
	value, err := sonic.Marshal(metadata)
	if err != nil {
		return errors.WithMessage(err, "failed to marshal service metadata")
	}

	// 创建租约 10s TTL
	lease, err := er.client.Grant(ctx, 10)
	if err != nil {
		return errors.WithMessage(err, "failed to create etcd lease")
	}
	er.leaseId = lease.ID

	_, err = er.client.Put(ctx, key, string(value), clientv3.WithLease(er.leaseId))
	if err != nil {
		return errors.WithMessage(err, "failed to register service instance in etcd")
	}

	// 设置自动续约
	keepAlive, err := er.client.KeepAlive(ctx, er.leaseId)
	if err != nil {
		return errors.WithMessage(err, "failed to set up keep-alive for etcd lease")
	}
	er.keepAliveChan = keepAlive

	go er.keepAlive()

	return nil
}

func (er *EtcdRegistry) Deregister(ctx context.Context, serviceName, instanceId string) error {
	if er.leaseId != 0 {
		_, err := er.client.Revoke(ctx, er.leaseId)
		if err != nil {
			return errors.WithMessage(err, "failed to revoke etcd lease")
		}
	}

	key := path.Join(er.discoveryPath, serviceName, instanceId)
	_, err := er.client.Delete(ctx, key)
	if err != nil {
		return errors.WithMessage(err, "failed to deregister service instance from etcd")
	}

	return nil
}

func (er *EtcdRegistry) Update(ctx context.Context, serviceName, instanceId string, metadata *ServiceMetadata) error {
	return er.Register(ctx, serviceName, instanceId, metadata)
}

func (er *EtcdRegistry) keepAlive() {
	for {
		leaseKeepAliveResponse, ok := <-er.keepAliveChan
		if !ok {
			zap.L().Warn("Etcd lease keep-alive channel closed")
			return
		}
		if leaseKeepAliveResponse == nil {
			zap.L().Warn("Etcd lease keep-alive response is nil")
			return
		}
	}
}

// EtcdRegistryBuilder etcd registry 构造器
type EtcdRegistryBuilder struct {
}

func (erb *EtcdRegistryBuilder) Build(conf any) (Registry, error) {
	cfg, ok := conf.(EtcdConfig)
	if !ok {
		return nil, errors.New("invalid config type for EtcdRegistry")
	}

	return NewEtcdRegistry(cfg.Endpoints, cfg.DialTimeout, cfg.DiscoveryPath)
}
