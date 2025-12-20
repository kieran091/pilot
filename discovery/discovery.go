package discovery

type Mode string

const (
	EtcdMode Mode = "etcd"
)

type Instance struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
}

type HTTPRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body"`
}

type RPCRoute struct {
	Service string `json:"service"`
	Method  string `json:"method"`
}

type Route struct {
	HTTPRoute HTTPRoute `json:"httpRoute"`
	RPCRoute  RPCRoute  `json:"rpcRoute"`
}

type Definition struct {
	Name  string  `json:"name"`
	Addr  string  `json:"addr"`
	Routes []Route `json:"routes"`
	PB    string  `json:"pb"`
}

type Registration struct {
	*Definition
	Instance *Instance
}

type Event struct {
	Type        Type
	ServiceInfo *Registration
}

type Type int

const (
	Added Type = iota
	Removed
)
