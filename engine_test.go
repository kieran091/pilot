package pilot

import (
	"context"
	"testing"
	"time"

	"github.com/kieran091/pilot/discovery"
	"github.com/kieran091/pilot/discovery/etcd"
	"github.com/stretchr/testify/require"
)

func TestNewEngine(t *testing.T) {
	cfg := Config{
		Server: &ServerConfig{
			Addr:           ":8000",
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   10 << 20,
		},
		Discovery: &ServiceDiscoveryConfig{
			Mode: discovery.EtcdMode,
			Etcd: &EtcdRegistryConfig{
				Endpoints:     []string{"127.0.0.1:2379"},
				DiscoveryPath: "test/server",
				DialTimeout:   5 * time.Second,
			},
		},
	}

	// create etcd watch
	// support: etcd | consul | nacos | consumer
	// the consumer watcher must impl etcdWatcher interface
	watcher, err := etcd.NewWatcher(
		cfg.Discovery.Etcd.Endpoints,
		cfg.Discovery.Etcd.DialTimeout,
		cfg.Discovery.Etcd.DiscoveryPath,
	)

	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, err := NewEngine(
		cfg,
		WithContext(ctx),
		WithWatcher(watcher),
	)

	require.NoError(t, err)

	// set middleware
	engine.Use(
		Recovery(),
		Log(),
	)

	if err := engine.Start(); err != nil {
		panic(err)
	}
}
