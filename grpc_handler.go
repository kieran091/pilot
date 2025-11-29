package pilot

import (
	"io"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type GrpcEventHandler struct {
	output    io.Writer
	err       error
	marshaler *jsonpb.Marshaler
}

func (h *GrpcEventHandler) OnResolveMethod(md *desc.MethodDescriptor) {}

func (h *GrpcEventHandler) OnSendHeaders(md metadata.MD) {}

func (h *GrpcEventHandler) OnReceiveHeaders(md metadata.MD) {}

func (h *GrpcEventHandler) OnReceiveResponse(msg proto.Message) {
	if h.marshaler == nil {
		h.marshaler = &jsonpb.Marshaler{
			EmitDefaults: true,
			OrigName:     true,
		}
	}
	jsonStr, err := h.marshaler.MarshalToString(msg)
	if err != nil {
		h.err = errors.WithMessage(err, "failed to marshal response message to JSON")
		return
	}
	if _, err := io.WriteString(h.output, jsonStr); err != nil && h.err == nil {
		h.err = errors.WithMessage(err, "failed to write response")
		return
	}
	if _, err := io.WriteString(h.output, "\n"); err != nil && h.err == nil {
		h.err = errors.WithMessage(err, "failed to write response")
	}
}

func (h *GrpcEventHandler) OnReceiveTrailers(stat *status.Status, md metadata.MD) {
	if stat.Code() != codes.OK && h.err == nil {
		h.err = stat.Err()
	}
}
