package pilot

import (
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
	"github.com/pkg/errors"
)

func fileDescriptorSet2FileDescriptor(fds *descriptor.FileDescriptorSet) ([]*desc.FileDescriptor, error) {
	filesMap, err := desc.CreateFileDescriptorsFromSet(fds)
	if err != nil {
		return nil, errors.WithMessage(err, "create file descriptors from set failed")
	}

	files := make([]*desc.FileDescriptor, 0, len(filesMap))
	for _, fd := range filesMap {
		files = append(files, fd)
	}

	return files, nil
}
