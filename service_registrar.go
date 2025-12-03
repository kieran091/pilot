package pilot

import (
	"bytes"
	"context"
	"encoding/base64"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// RegisterOption defines options for service registration
type RegisterOption func(registrar *ServiceRegistrar)

// WithFile adds a proto file to be compiled for service registration
func WithFile(filePath string) RegisterOption {
	return func(sr *ServiceRegistrar) {
		sr.files = append(sr.files, filePath)
	}
}

// WithProtoPath adds a proto import path for the compiler to resolve imports
func WithProtoPath(protoPath string) RegisterOption {
	return func(sr *ServiceRegistrar) {
		sr.protoPaths = append(sr.protoPaths, protoPath)
	}
}

type ServiceRegistrar struct {
	serviceName string
	addr        string
	instanceId  string

	registry Registry

	files      []string
	protoPaths []string
	compiler   *protocompile.Compiler
}

func NewRegister(serviceName, listenOn string, registry Registry) *ServiceRegistrar {
	return &ServiceRegistrar{
		serviceName: serviceName,
		addr:        figureOutListenOn(listenOn),
		instanceId:  getInstanceId(),

		registry: registry,

		files:      make([]string, 0, 5),
		protoPaths: make([]string, 0, 5),
		compiler: &protocompile.Compiler{
			Resolver:       &protocompile.SourceResolver{},
			SourceInfoMode: protocompile.SourceInfoStandard,
		},
	}
}

// Register compiles the proto files, extracts HTTP rules, and registers the service
func (sr *ServiceRegistrar) Register(ctx context.Context, opts ...RegisterOption) error {
	// Apply options
	for _, opt := range opts {
		opt(sr)
	}

	// Compile the proto files
	fds, err := sr.compileProto()
	if err != nil {
		return err
	}

	// Extract HTTP rules
	rules, err := sr.extractRules(fds)
	if err != nil {
		return err
	}

	// Serialize the FileDescriptorSet to proto DSL
	protoDsl, err := proto.Marshal(fds)
	if err != nil {
		return err
	}

	serviceMetadata := ServiceMetadata{
		Name:     sr.serviceName,
		Addr:     sr.addr,
		Rules:    rules,
		ProtoDSL: base64.StdEncoding.EncodeToString(protoDsl),
	}

	return sr.registry.Register(ctx, sr.serviceName, sr.instanceId, &serviceMetadata)
}

// Deregister removes the service registration
func (sr *ServiceRegistrar) Deregister(ctx context.Context) error {
	return sr.registry.Deregister(ctx, sr.serviceName, sr.instanceId)
}

// compileProto compiles the added proto files and returns the serialized
func (sr *ServiceRegistrar) compileProto() (*descriptorpb.FileDescriptorSet, error) {
	if len(sr.files) == 0 {
		return nil, errors.New("no proto files to compile")
	}

	// Set up custom resolver if proto paths are provided
	if len(sr.protoPaths) != 0 {
		// Set up a custom resolver that searches the provided proto paths
		baseResolver := protocompile.ResolverFunc(func(name string) (protocompile.SearchResult, error) {
			if filepath.IsAbs(name) {
				return openProto(name)
			}

			for _, base := range sr.protoPaths {
				candidate := filepath.Join(base, name)
				searchResult, err := openProto(candidate)
				if err == nil {
					return searchResult, nil
				}
				if !errors.Is(err, fs.ErrNotExist) {
					return protocompile.SearchResult{}, err
				}
			}
			return protocompile.SearchResult{}, errors.Errorf("cannot locate import %q in any of the proto paths", name)
		})

		sr.compiler.Resolver = protocompile.WithStandardImports(baseResolver)
	}

	// Compile the proto files
	linkedFiles, err := sr.compiler.Compile(context.TODO(), sr.files...)
	if err != nil {
		return nil, err
	}

	fds := &descriptorpb.FileDescriptorSet{
		File: make([]*descriptorpb.FileDescriptorProto, 0, len(linkedFiles)),
	}
	added := make(map[string]struct{})

	// Recursive function to add file descriptors and their imports
	var addFile func(fd protoreflect.FileDescriptor)
	addFile = func(fd protoreflect.FileDescriptor) {
		name := string(fd.FullName())
		if _, exists := added[name]; exists {
			return
		}

		imports := fd.Imports()
		for i := range imports.Len() {
			addFile(imports.Get(i).FileDescriptor)
		}

		added[name] = struct{}{}
		fds.File = append(fds.File, protodesc.ToFileDescriptorProto(fd))
	}

	return fds, nil
}

// extractRules extracts HTTP rules from the compiled FileDescriptorSet
func (sr *ServiceRegistrar) extractRules(fd *descriptorpb.FileDescriptorSet) ([]HTTPRule, error) {
	// Convert FileDescriptorSet to FileDescriptors
	fileDescriptors, err := fileDescriptorSet2FileDescriptor(fd)
	if err != nil {
		return nil, err
	}

	// Extract HTTP rules from services and methods
	var rules []HTTPRule
	for _, fileDescriptor := range fileDescriptors {
		services := fileDescriptor.GetServices()
		for _, service := range services {
			methods := service.GetMethods()
			for _, method := range methods {
				httpRule, err := extractHTTPRule(method)
				if err != nil {
					continue
				}

				rules = append(rules, httpRule...)
			}
		}
	}

	return rules, nil
}

// openProto reads a proto file from the given path and returns a SearchResult
func openProto(path string) (protocompile.SearchResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return protocompile.SearchResult{}, err
	}

	return protocompile.SearchResult{
		Source: bytes.NewReader(data),
	}, nil
}

// figureOutListenOn determines the appropriate listen address
func figureOutListenOn(listenOn string) string {
	fields := strings.Split(listenOn, ":")
	if len(fields) == 0 {
		return listenOn
	}

	host := fields[0]
	if len(host) > 0 && host != "0.0.0.0" {
		return listenOn
	}

	ip := os.Getenv("POD_IP")
	if len(ip) == 0 {
		ip = internalIp()
	}
	if len(ip) == 0 {
		return listenOn
	}

	return strings.Join(append([]string{ip}, fields[1:]...), ":")
}

// internalIp retrieves the first non-loopback IPv4 address of the machine
func internalIp() string {
	infs, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, inf := range infs {
		if isEthDown(inf.Flags) || isLoopback(inf.Flags) {
			continue
		}

		addrs, err := inf.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
	}

	return ""
}

// isEthDown checks if the network interface is down
func isEthDown(f net.Flags) bool {
	return f&net.FlagUp != net.FlagUp
}

// isLoopback checks if the network interface is a loopback
func isLoopback(f net.Flags) bool {
	return f&net.FlagLoopback == net.FlagLoopback
}

// getInstanceId generates a UUIDv7 instance ID
func getInstanceId() string {
	uuidV7, err := uuid.NewV7()
	if err != nil {
		return uuid.New().String()
	}
	return uuidV7.String()
}
