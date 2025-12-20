package pilot

import (
	"fmt"
	"strings"

	"github.com/kieran091/pilot/discovery"
	"github.com/pkg/errors"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)


func extractRoute(method protoreflect.MethodDescriptor) ([]discovery.Route, error) {
	opts := method.Options()
	if opts == nil {
		return nil, nil
	}

	optsData, err := proto.Marshal(opts)
	if err != nil {
		return nil, nil
	}

	methodOptions := &descriptorpb.MethodOptions{}
	if err := proto.Unmarshal(optsData, methodOptions); err != nil {
		return nil, nil
	}

	if !proto.HasExtension(methodOptions, annotations.E_Http) {
		return nil, nil
	}

	ext := proto.GetExtension(methodOptions, annotations.E_Http)
	httpRoute, ok := ext.(*annotations.HttpRule)
	if !ok {
		return nil, nil
	}

	rules := make([]discovery.Route, 0)

	rpcRoute, err := parseRPCRoute(string(method.FullName()))
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse RPC rule")
	}

	mainHTTPRoute, err := parseHTTPRoute(httpRoute)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse Server rule")
	}
	if mainHTTPRoute != nil {
		rules = append(rules, discovery.Route{
			HTTPRoute: *mainHTTPRoute,
			RPCRoute:  *rpcRoute,
		})
	}

	for _, additionalRule := range httpRoute.AdditionalBindings {
		additionalHTTPRoute, err := parseHTTPRoute(additionalRule)
		if err != nil {
			return nil, fmt.Errorf("failed to parse additional binding: %w", err)
		}
		if additionalHTTPRoute != nil {
			rules = append(rules, discovery.Route{
				HTTPRoute: *additionalHTTPRoute,
				RPCRoute:  *rpcRoute,
			})
		}
	}

	return rules, nil
}

func parseHTTPRoute(rule *annotations.HttpRule) (*discovery.HTTPRoute, error) {
	if rule == nil {
		return nil, nil
	}

	httpRoute := &discovery.HTTPRoute{
		Body: rule.Body,
	}

	switch pattern := rule.Pattern.(type) {
	case *annotations.HttpRule_Get:
		httpRoute.Method = "GET"
		httpRoute.Path = pattern.Get
	case *annotations.HttpRule_Post:
		httpRoute.Method = "POST"
		httpRoute.Path = pattern.Post
	case *annotations.HttpRule_Put:
		httpRoute.Method = "PUT"
		httpRoute.Path = pattern.Put
	case *annotations.HttpRule_Delete:
		httpRoute.Method = "DELETE"
		httpRoute.Path = pattern.Delete
	case *annotations.HttpRule_Patch:
		httpRoute.Method = "PATCH"
		httpRoute.Path = pattern.Patch
	case *annotations.HttpRule_Custom:
		httpRoute.Method = pattern.Custom.Kind
		httpRoute.Path = pattern.Custom.Path
	default:
		return nil, fmt.Errorf("unknown Server rule pattern type")
	}

	return httpRoute, nil
}

func parseRPCRoute(fullMethod string) (*discovery.RPCRoute, error) {
	service, method, err := parseFullMethod(fullMethod)
	if err != nil {
		return nil, err
	}

	return &discovery.RPCRoute{
		Service: service,
		Method:  method,
	}, nil
}

// parseFullMethod splits a full method name into service and method components.
// eq: "/package.Service/Method" or "package.Service.Method"
func parseFullMethod(fullMethod string) (serviceName, method string, err error) {
	fullMethod = strings.TrimPrefix(fullMethod, "/")

	// Try to split by '/' first
	if idx := strings.LastIndex(fullMethod, "/"); idx > 0 {
		return fullMethod[:idx], fullMethod[idx+1:], nil
	}

	// Fallback to splitting by '.'
	if idx := strings.LastIndex(fullMethod, "."); idx > 0 {
		return fullMethod[:idx], fullMethod[idx+1:], nil
	}

	return "", "", errors.Errorf("invalid method name format: %s", fullMethod)
}
