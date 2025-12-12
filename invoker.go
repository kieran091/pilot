package pilot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// MethodInfo holds information about a gRPC method, including its descriptors and message pools.
type MethodInfo struct {
	MethodDesc protoreflect.MethodDescriptor
	InputDesc  protoreflect.MessageDescriptor
	OutputDesc protoreflect.MessageDescriptor
	FullMethod string

	reqPool  sync.Pool
	respPool sync.Pool
}

// getRequest retrieves a request message from the pool.
func (m *MethodInfo) getRequest() *dynamicpb.Message {
	return m.reqPool.Get().(*dynamicpb.Message)
}

// putRequest returns a request message to the pool.
func (m *MethodInfo) putRequest(msg *dynamicpb.Message) {
	msg.Reset()
	m.reqPool.Put(msg)
}

// getResponse retrieves a response message from the pool.
func (m *MethodInfo) getResponse() *dynamicpb.Message {
	return m.respPool.Get().(*dynamicpb.Message)
}

// putResponse returns a response message to the pool.
func (m *MethodInfo) putResponse(msg *dynamicpb.Message) {
	msg.Reset()
	m.respPool.Put(msg)
}

// GRPCInvoker is a dynamic gRPC client that can invoke methods based on protobuf descriptors.
type GRPCInvoker struct {
	conn  *grpc.ClientConn
	addr  string
	files *protoregistry.Files

	methodCache sync.Map // map[string]*MethodInfo

	marshalOpts   protojson.MarshalOptions
	unmarshalOpts protojson.UnmarshalOptions

	bufferPool sync.Pool // bytes buffer pool for request/response
}

func NewGRPCInvoker(addr string, fds *descriptorpb.FileDescriptorSet) (*GRPCInvoker, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: false,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(24*1024*1024),
			grpc.MaxCallRecvMsgSize(24*1024*1024),
		),
		grpc.WithDefaultServiceConfig(`{
			"loadBalancingPolicy": "round_robin",
            "methodConfig": [{
                "name": [{"service": ""}],
                "waitForReady": true,
                "retryPolicy": {
                    "maxAttempts": 3,
                    "initialBackoff": "0.1s",
                    "maxBackoff": "1s",
                    "backoffMultiplier": 1.3,
                    "retryableStatusCodes": ["UNAVAILABLE", "RESOURCE_EXHAUSTED", "INTERNAL"]
				}
			}]
		}`),
		grpc.WithConnectParams(grpc.ConnectParams{MinConnectTimeout: 10 * time.Second}),
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create gRPC client connection")
	}

	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create proto registry from file descriptor set")
	}

	return &GRPCInvoker{
		conn:  conn,
		addr:  addr,
		files: files,
		marshalOpts: protojson.MarshalOptions{
			EmitUnpopulated: true,
			UseProtoNames:   true,
		},
		unmarshalOpts: protojson.UnmarshalOptions{
			DiscardUnknown: true,
		},
		bufferPool: sync.Pool{
			New: func() any {
				b := make([]byte, 0, 4*1024)
				return &b
			},
		},
	}, nil
}

// Invoke calls a gRPC method by its name with the given input in JSON format.
// It returns the response in JSON format.
func (inv *GRPCInvoker) Invoke(ctx context.Context, service, method string, input []byte) ([]byte, error) {
	if _, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	methodInfo, err := inv.getMethodInfo(service, method)
	if err != nil {
		return nil, err
	}

	req := methodInfo.getRequest()
	defer methodInfo.putRequest(req)

	if len(input) > 0 {
		if err := inv.unmarshalOpts.Unmarshal(input, req); err != nil {
			return nil, errors.WithMessage(err, "failed to unmarshal request JSON")
		}
	}

	resp := methodInfo.getResponse()
	defer methodInfo.putResponse(resp)

	err = inv.conn.Invoke(ctx, methodInfo.FullMethod, req, resp)
	if err != nil {
		return nil, errors.WithMessage(err, "filed to invoke gRPC method")
	}

	output, err := inv.marshalOpts.Marshal(resp)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal response JSON")
	}

	return output, nil
}

// getMethodInfo retrieves method information for a given method name, utilizing caching for efficiency.
// The method name can be in the format "/package.Service/Method" or "package.Service.Method".
func (inv *GRPCInvoker) getMethodInfo(service, method string) (*MethodInfo, error) {
	fullMethod := fmt.Sprintf("/%s/%s", service, method)
	if cached, ok := inv.methodCache.Load(fullMethod); ok {
		return cached.(*MethodInfo), nil
	}

	desc, err := inv.files.FindDescriptorByName(protoreflect.FullName(service))
	if err != nil {
		return nil, errors.WithMessagef(err, "service %s not found", service)
	}

	serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, errors.Errorf("%s is not a service descriptor", service)
	}

	methodDesc := serviceDesc.Methods().ByName(protoreflect.Name(method))
	if methodDesc == nil {
		return nil, errors.Errorf("method %s not found in service %s", method, service)
	}

	info := &MethodInfo{
		MethodDesc: methodDesc,
		InputDesc:  methodDesc.Input(),
		OutputDesc: methodDesc.Output(),
		FullMethod: fullMethod,
	}
	info.reqPool = sync.Pool{
		New: func() any {
			return dynamicpb.NewMessage(info.InputDesc)
		},
	}
	info.respPool = sync.Pool{
		New: func() any {
			return dynamicpb.NewMessage(info.OutputDesc)
		},
	}

	actual, _ := inv.methodCache.LoadOrStore(fullMethod, info)
	return actual.(*MethodInfo), nil
}

// Close closes the underlying gRPC connection.
func (inv *GRPCInvoker) Close() error {
	return inv.conn.Close()
}
