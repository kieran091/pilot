package pilot

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type HTTPRule struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body"`
}

type RPCRule struct {
	Service string `json:"service"`
	Method  string `json:"method"`
}

type Rule struct {
	HTTPRule HTTPRule `json:"http_rule"`
	RPCRule  RPCRule  `json:"rpc_rule"`
}

func extractRule(method protoreflect.MethodDescriptor) ([]Rule, error) {
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
	httpRule, ok := ext.(*annotations.HttpRule)
	if !ok {
		return nil, nil
	}

	rules := make([]Rule, 0)

	rpcRule, err := parseRPCRule(string(method.FullName()))
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse RPC rule")
	}

	mainHTTPRule, err := parseHTTPRule(httpRule)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse HTTP rule")
	}
	if mainHTTPRule != nil {
		rules = append(rules, Rule{
			HTTPRule: *mainHTTPRule,
			RPCRule:  *rpcRule,
		})
	}

	for _, additionalRule := range httpRule.AdditionalBindings {
		additionalHTTPRule, err := parseHTTPRule(additionalRule)
		if err != nil {
			return nil, fmt.Errorf("failed to parse additional binding: %w", err)
		}
		if additionalHTTPRule != nil {
			rules = append(rules, Rule{
				HTTPRule: *additionalHTTPRule,
				RPCRule:  *rpcRule,
			})
		}
	}

	return rules, nil
}

func parseHTTPRule(rule *annotations.HttpRule) (*HTTPRule, error) {
	if rule == nil {
		return nil, nil
	}

	httpRule := &HTTPRule{
		Body: rule.Body,
	}

	switch pattern := rule.Pattern.(type) {
	case *annotations.HttpRule_Get:
		httpRule.Method = "GET"
		httpRule.Path = pattern.Get
	case *annotations.HttpRule_Post:
		httpRule.Method = "POST"
		httpRule.Path = pattern.Post
	case *annotations.HttpRule_Put:
		httpRule.Method = "PUT"
		httpRule.Path = pattern.Put
	case *annotations.HttpRule_Delete:
		httpRule.Method = "DELETE"
		httpRule.Path = pattern.Delete
	case *annotations.HttpRule_Patch:
		httpRule.Method = "PATCH"
		httpRule.Path = pattern.Patch
	case *annotations.HttpRule_Custom:
		httpRule.Method = pattern.Custom.Kind
		httpRule.Path = pattern.Custom.Path
	default:
		return nil, fmt.Errorf("unknown HTTP rule pattern type")
	}

	return httpRule, nil
}

func parseRPCRule(fullMethod string) (*RPCRule, error) {
	service, method, err := parseFullMethod(fullMethod)
	if err != nil {
		return nil, err
	}

	return &RPCRule{
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
