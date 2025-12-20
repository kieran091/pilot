package pilot

import (
	"time"

	"github.com/kieran091/pilot/discovery"
)

type ServerConfig struct {
	Address        string        `json:"address" yaml:"address"`
	ReadTimeout    time.Duration `json:"readTimeout" yaml:"readTimeout"`
	WriteTimeout   time.Duration `json:"writeTimeout" yaml:"writeTimeout"`
	MaxHeaderBytes int           `json:"maxHeaderBytes" yaml:"maxHeaderBytes"`
	MaxBodyBytes   int           `json:"maxBodyBytes" yaml:"maxBodyBytes"`
}

type EtcdRegistryConfig struct {
	Endpoints     []string      `json:"endpoints" yaml:"endpoints"`
	DiscoveryPath string        `json:"discoveryPath" yaml:"discoveryPath"`
	DialTimeout   time.Duration `json:"dialTimeout" yaml:"dialTimeout"`
}

type ServiceDiscoveryConfig struct {
	Mode discovery.Mode      `json:"mode" yaml:"mode"`
	Etcd *EtcdRegistryConfig `json:"etcd" yaml:"etcd"`
}

type Config struct {
	Server    *ServerConfig           `json:"server" yaml:"server"`
	Discovery *ServiceDiscoveryConfig `json:"discovery" yaml:"discovery"`
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
	if c.Server.Address == "" {
		c.Server.Address = defaultHTTPAddr
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = defaultHTTPReadTimeout
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = defaultHTTPWriteTimeout
	}
	if c.Server.MaxBodyBytes == 0 {
		c.Server.MaxBodyBytes = defaultHTTPMaxBodyBytes
	}

	if c.Discovery.Mode == "" {
		c.Discovery.Mode = defaultDiscoveryMode

		if c.Discovery.Etcd == nil {
			etcdCfg := &EtcdRegistryConfig{
				Endpoints:     []string{defaultEtcdEndpoints},
				DiscoveryPath: defaultDiscoveryPath,
				DialTimeout:   defaultEtcdDialTimeout,
			}
			c.Discovery.Etcd = etcdCfg
		}
	}
}
