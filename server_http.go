package pilot

import (
	"net/http"
	"strings"
)

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := NewContext(w, req)

	routeTree, exists := r.trees.getTree(strings.ToUpper(req.Method))
	if !exists {
		http.NotFound(w, req)
		return
	}

	route, params, exists := routeTree.Lookup(req.URL.Path)
	if !exists {
		http.NotFound(w, req)
		return
	}

	for k, v := range params {
		ctx.SetParams(k, v)
	}

	ctx.SetService(route.service)
	ctx.SetMethodName(route.methodName)

	ctx.handlers = make([]HandlerFunc, len(r.globalHandlers))
	copy(ctx.handlers, r.globalHandlers)

	ctx.handlers = append(ctx.handlers, func(ctx *Context) {
		// TODO: invoke the service method using route.invoke
	})

	ctx.Next()
}
