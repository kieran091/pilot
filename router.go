package pilot

import (
	"strings"
	"sync"

	"github.com/pkg/errors"
)

type Route struct {
	ServiceName string
	FullMethod  string
	Rule        HTTPRule
}

type Router struct {
	routeTree  *RouteTree[*Route]
	routeIndex map[string]map[string]struct{}
	pathIndex  map[string]struct{}
	mu         sync.RWMutex
}

func NewRouter() *Router {
	return &Router{
		routeTree:  NewRouteTree[*Route](),
		routeIndex: map[string]map[string]struct{}{},
		pathIndex:  map[string]struct{}{},
		mu:         sync.RWMutex{},
	}
}

func (r *Router) insert(serviceInfo *ServiceInfo) error {
	if serviceInfo == nil || len(strings.TrimSpace(serviceInfo.Name)) == 0 {
		return errors.New("invalid service info")
	}

	return nil
}

func (r *Router) delete(serviceInfo *ServiceInfo) error {
	return nil
}

func (r *Router) Close() {}
