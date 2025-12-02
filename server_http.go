package pilot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"google.golang.org/grpc/metadata"
)

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := NewContext(w, req)

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
		c.SetParams(k, v)
	}

	c.SetService(route.service)
	c.SetMethodName(route.methodName)

	c.handlers = make([]HandlerFunc, len(r.globalHandlers))
	copy(c.handlers, r.globalHandlers)

	c.handlers = append(c.handlers, func(c *Context) {
		requestData, err := buildRequestData(c.Request, params, route.bodyField)
		if err != nil {
			c.Abort()
			c.AddError(err)
			return
		}

		c.ctx = metadata.NewOutgoingContext(c.ctx, buildOutgoingMD(c))
		responseData, err := route.invoke(c, c.Request.URL.Path, requestData)
		if err != nil {
			c.Abort()
			c.AddError(err)

			status, res := buildErrorResult(err)
			c.Writer.WriteJSON(status, res)
			return
		}

		var data any
		if err := sonic.Unmarshal(responseData, &data); err != nil {
			data = json.RawMessage(responseData)
		}

		c.Writer.WriteJSON(http.StatusOK, buildResult("ok", data))
	})

	c.Next()
}

func buildRequestData(req *http.Request, params map[string]string, bodyField string) ([]byte, error) {
	requestMap := make(map[string]any)

	for key, value := range params {
		requestMap[key] = value
	}

	for key, values := range req.URL.Query() {
		if len(values) == 1 {
			requestMap[key] = values[0]
		} else {
			requestMap[key] = values
		}
	}

	if req.Method != "GET" && req.Method != "DELETE" {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = req.Body.Close()
		}()

		if len(body) > 0 {
			if bodyField == "*" {
				var bodyMap map[string]any
				if err := sonic.Unmarshal(body, &bodyMap); err != nil {
					return nil, err
				}

				for k, v := range bodyMap {
					requestMap[k] = v
				}
			} else if bodyField != "" {
				var bodyData any
				if err := sonic.Unmarshal(body, &bodyData); err != nil {
					return nil, err
				}

				requestMap[bodyField] = bodyData
			}
		}
	}

	return sonic.Marshal(requestMap)
}

func buildOutgoingMD(ctx *Context) metadata.MD {
	md := metadata.New(nil)

	for k, v := range ctx.keys {
		md.Append(k, fmt.Sprintf("%v", v))
	}

	return md
}
