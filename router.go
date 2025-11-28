package pilot

import (
	"sync"
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
