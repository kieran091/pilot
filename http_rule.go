package pilot

import (
	"fmt"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type HTTPRule struct {
	Method string
	Path   string
	Body   string
}

func extractHTTPRule(method protoreflect.MethodDescriptor) ([]HTTPRule, error) {
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

	rules := make([]HTTPRule, 0)

	mainRule, err := parseHTTPRule(httpRule)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTTP rule: %w", err)
	}
	if mainRule != nil {
		rules = append(rules, *mainRule)
	}

	for _, additionalRule := range httpRule.AdditionalBindings {
		rule, err := parseHTTPRule(additionalRule)
		if err != nil {
			return nil, fmt.Errorf("failed to parse additional binding: %w", err)
		}
		if rule != nil {
			rules = append(rules, *rule)
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
