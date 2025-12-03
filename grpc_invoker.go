package pilot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/descriptorpb"
)

type GRPCInvoker struct {
	conn *grpc.ClientConn
	addr string
	grpcurl.DescriptorSource
}

func NewGRPCInvoker(addr string, fileDescriptorSet *descriptorpb.FileDescriptorSet) (*GRPCInvoker, error) {
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

	//fileDescriptors, err := fileDescriptorSet2FileDescriptor(fileDescriptorSet)
	//if err != nil {
	//	return nil, errors.WithMessage(err, "failed to create file descriptor from file descriptor set")
	//}
	//
	//descriptorSource, err := grpcurl.DescriptorSourceFromFileDescriptors(fileDescriptors...)
	//if err != nil {
	//	return nil, errors.WithMessage(err, "failed to create descriptor source from file descriptors")
	//}

	return &GRPCInvoker{
		conn: conn,
		addr: addr,
		//DescriptorSource: descriptorSource,
	}, nil
}

func (inv *GRPCInvoker) Invoke(ctx context.Context, methodName string, input []byte) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	var in io.Reader
	var out bytes.Buffer
	if len(input) > 0 {
		in = bytes.NewReader(input)
	} else {
		in = bytes.NewBufferString("{}")
	}

	handler := &GrpcEventHandler{
		output: &out,
	}

	md, _ := metadata.FromOutgoingContext(ctx)
	headers := make([]string, 0)
	for k, v := range md {
		for _, val := range v {
			headers = append(headers, fmt.Sprintf("%s: %s", k, val))
		}
	}

	rf, _, err := grpcurl.RequestParserAndFormatter(
		grpcurl.FormatJSON,
		inv.DescriptorSource,
		in,
		grpcurl.FormatOptions{},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create request parser and formatter")
	}

	err = grpcurl.InvokeRPC(
		ctx,
		inv.DescriptorSource,
		inv.conn,
		methodName,
		headers,
		handler,
		rf.Next,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to invoke gRPC method")
	}
	if handler.err != nil {
		return nil, handler.err
	}

	return bytes.TrimRight(out.Bytes(), "\n"), nil
}

func (inv *GRPCInvoker) Close() error {
	return inv.conn.Close()
}
