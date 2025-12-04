package pilot

type Mode string

const (
	Etcd Mode = "etcd"
)

type ServiceInstance struct {
	Id   string
	Addr string
}

type ServiceMetadata struct {
	Name  string `json:"name"`
	Addr  string `json:"addr"`
	Rules []Rule `json:"rules"`
	Pb    string `json:"pb"`
}

type ServiceInfo struct {
	*ServiceMetadata
	Instance *ServiceInstance
}

type ServiceEvent struct {
	Type        EventType
	ServiceInfo *ServiceInfo
}

type EventType int

const (
	EventAdd EventType = iota
	EventDelete
)
