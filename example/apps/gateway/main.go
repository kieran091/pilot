package main

import (
	"context"
	"log"
	"time"

	"github.com/kieran091/pilot"
)

func main() {
	cfg := pilot.Config{
		HTTP: pilot.HTTPConfig{
			Addr:           ":8000",
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   10 << 20,
		},
		Discovery: pilot.DiscoveryConfig{
			Mode: pilot.Etcd,
			Etcd: &pilot.EtcdConfig{
				Endpoints:     []string{"127.0.0.1:2379"},
				DiscoveryPath: "test/server",
				DialTimeout:   5 * time.Second,
			},
		},
	}

	watcher, err := pilot.NewEtcdWatcher(
		cfg.Discovery.Etcd.Endpoints,
		cfg.Discovery.Etcd.DialTimeout,
		cfg.Discovery.Etcd.DiscoveryPath,
	)
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, err := pilot.NewEngine(cfg, pilot.WithContext(ctx), pilot.WithWatcher(watcher))
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
