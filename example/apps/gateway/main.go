package main

import (
	"context"
	"log"
	"time"

	"github.com/kieran091/pilot"
	"github.com/kieran091/pilot/discovery"
	"github.com/kieran091/pilot/discovery/etcd"
)

func main() {
	cfg := pilot.Config{
		Server: &pilot.ServerConfig{
			Addr:           ":8000",
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   10 << 20,
		},
		Discovery: &pilot.ServiceDiscoveryConfig{
			Mode: discovery.EtcdMode,
			Etcd: &pilot.EtcdRegistryConfig{
				Endpoints:     []string{"127.0.0.1:2379"},
				DiscoveryPath: "test/server",
				DialTimeout:   5 * time.Second,
			},
		},
	}

	watcher, err := etcd.NewWatcher(
		cfg.Discovery.Etcd.Endpoints,
		cfg.Discovery.Etcd.DialTimeout,
		cfg.Discovery.Etcd.DiscoveryPath,
	)
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, err := pilot.NewEngine(&cfg, pilot.WithContext(ctx), pilot.WithWatcher(watcher))
	if err != nil {
		log.Fatalln(err)
	}

	engine.Use(
		pilot.Recovery(),
		pilot.Log(),
	)

	if err := engine.Start(); err != nil {
		log.Fatalln(err)
	}
}
