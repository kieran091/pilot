package pilot

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

type InvokerFunc func(ctx *Context, key string, input []byte) ([]byte, error)

type routeTrees map[string]*RouteTree[*Route]

func (mt routeTrees) getTree(method string) (*RouteTree[*Route], bool) {
	methodTree, exists := mt[method]
	return methodTree, exists
}

type Route struct {
	service    string
	methodName string
	bodyField  string
	//handlers   []HandlerFunc
	invoke InvokerFunc
}

type Router struct {
	trees routeTrees

	routeIndex map[string]map[string]string // service name -> route path -> method name
	pathIndex  map[string]struct{}

	globalHandlers []HandlerFunc

	serviceRegistry ServiceRegistry
	mu              sync.RWMutex
	closed          bool
}

func NewRouter() *Router {
	return &Router{
		trees:           make(routeTrees, 9),
		routeIndex:      make(map[string]map[string]string),
		pathIndex:       make(map[string]struct{}),
		globalHandlers:  make([]HandlerFunc, 0),
		serviceRegistry: make(ServiceRegistry),
	}
}

func (r *Router) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}

	r.closed = true
	endpoints := make([]*ServiceEndpoint, 0, len(r.serviceRegistry))
	for _, endpoint := range r.serviceRegistry {
		endpoints = append(endpoints, endpoint)
	}
	r.mu.Unlock()

	for _, endpoint := range endpoints {
		endpoint.Close()
	}

	r.mu.Lock()
	r.trees = make(routeTrees, 9)
	r.routeIndex = make(map[string]map[string]string)
	r.pathIndex = make(map[string]struct{})
	r.serviceRegistry = make(ServiceRegistry)
	r.globalHandlers = nil
	r.mu.Unlock()
}

// insert inserts service info into the router.
func (r *Router) insert(serviceInfo *ServiceInfo) error {
	if r.isClosed() {
		return errors.New("router is closed")
	}

	if serviceInfo == nil || len(strings.TrimSpace(serviceInfo.Name)) == 0 {
		return errors.New("invalid service info")
	}

	r.mu.Lock()
	endpoint, exists := r.serviceRegistry[serviceInfo.Name]
	if !exists {
		endpoint = &ServiceEndpoint{
			replicas: 150,
		}
		r.serviceRegistry[serviceInfo.Name] = endpoint
	}
	needToInsertRoute := len(r.routeIndex[serviceInfo.Name]) == 0
	r.mu.Unlock()

	// update service instances
	endpoint.UpdateInstances(serviceInfo.Instances, serviceInfo.FileDescriptorSet)

	if needToInsertRoute {
		for _, rule := range serviceInfo.Rules {
			routePath := normalizePath(rule.HTTPRule.Path)

			r.mu.Lock()
			methodUpper := strings.ToUpper(rule.HTTPRule.Method)
			routeTree, exists := r.trees.getTree(methodUpper)
			if !exists {
				routeTree = NewRouteTree[*Route]()
				r.trees[methodUpper] = routeTree
			}

			err := routeTree.Insert(routePath, &Route{
				service:    rule.RPCRule.Service,
				methodName: rule.RPCRule.Method,
				bodyField:  rule.HTTPRule.Body,
				invoke:     endpoint.GetInvoker(rule.RPCRule.Service, rule.RPCRule.Method),
			})
			if err != nil {
				// TODO log error
				continue
			}

			// update route index
			if _, ok := r.routeIndex[serviceInfo.Name]; !ok {
				r.routeIndex[serviceInfo.Name] = make(map[string]string)
			}
			r.routeIndex[serviceInfo.Name][routePath] = methodUpper
			r.pathIndex[routePath] = struct{}{}
			r.mu.Unlock()
		}
	}
	return nil
}

func (r *Router) delete(serviceInfo *ServiceInfo) error {
	if r.isClosed() {
		return errors.New("router is closed")
	}
	if serviceInfo == nil || len(strings.TrimSpace(serviceInfo.Name)) == 0 {
		return errors.New("invalid service info")
	}

	r.mu.Lock()
	routeIndex := r.routeIndex[serviceInfo.Name]
	for routePath, method := range routeIndex {
		routeTree, exists := r.trees.getTree(method)
		if !exists {
			// TODO log error
			continue
		}
		ok := routeTree.Delete(routePath)
		if !ok {
			// TODO log error
			continue
		}
	}

	endpoint := r.serviceRegistry[serviceInfo.Name]
	endpoint.Close()

	delete(r.routeIndex, serviceInfo.Name)
	delete(r.serviceRegistry, serviceInfo.Name)
	r.mu.Unlock()

	return nil
}

// use adds global middleware to the router.
func (r *Router) use(mw ...HandlerFunc) {
	if len(mw) == 0 {
		return
	}

	r.mu.Lock()
	r.globalHandlers = append(r.globalHandlers, mw...)
	r.mu.Unlock()
}

func (r *Router) isClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.closed
}

func normalizePath(p string) string {
	clean := strings.TrimSpace(p)

	clean = strings.TrimPrefix(clean, "/")
	clean = path.Clean("/" + clean)
	return fmt.Sprintf("%s", clean)
}
