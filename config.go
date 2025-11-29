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

const (
	defaultHTTPAddr         = ":8080"
	defaultHTTPReadTimeout  = 15 * time.Second
	defaultHTTPWriteTimeout = 15 * time.Second
	defaultHTTPMaxBodyBytes = 10 << 20
	defaultDiscoveryMode    = "etcd"
	defaultEtcdEndpoints    = "localhost:2379"
	defaultDiscoveryPath    = "/services/"
	defaultEtcdDialTimeout  = 5 * time.Second
)

func (c *Config) setDefaults() {
	if c.HTTP.Addr == "" {
		c.HTTP.Addr = defaultHTTPAddr
	}
	if c.HTTP.ReadTimeout == 0 {
		c.HTTP.ReadTimeout = defaultHTTPReadTimeout
	}
	if c.HTTP.WriteTimeout == 0 {
		c.HTTP.WriteTimeout = defaultHTTPWriteTimeout
	}
	if c.HTTP.MaxBodyBytes == 0 {
		c.HTTP.MaxBodyBytes = defaultHTTPMaxBodyBytes
	}

	if c.Discovery.Mode == "" {
		c.Discovery.Mode = defaultDiscoveryMode

		if c.Discovery.Etcd == nil {
			etcdCfg := &EtcdConfig{
				Endpoints:     []string{defaultEtcdEndpoints},
				DiscoveryPath: defaultDiscoveryPath,
				DialTimeout:   defaultEtcdDialTimeout,
			}
			c.Discovery.Etcd = etcdCfg
		}
	}
}
