package pilot

import "time"

type HTTPConfig struct {
	Addr           string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int
	MaxBodyBytes   int
}

type EtcdConfig struct {
	Endpoints     []string
	DiscoveryPath string
	DialTimeout   time.Duration
}

type DiscoveryConfig struct {
	Mode string
	Etcd *EtcdConfig
}

type Config struct {
	HTTP      HTTPConfig
	Discovery DiscoveryConfig
}
