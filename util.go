package pilot

import (
	"net"
	"os"
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/google/uuid"
	"github.com/jhump/protoreflect/desc"
	"github.com/pkg/errors"
)

func splitPath(path string) []string {
	p := path
	if p == "" {
		return []string{""}
	}

	p = strings.Trim(p, " ")

	if idx := strings.IndexByte(p, '?'); idx >= 0 {
		p = p[:idx]
	}
	if idx := strings.IndexByte(p, '#'); idx >= 0 {
		p = p[:idx]
	}

	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if p == "/" {
		return []string{""}
	}
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p
	}

	segs := strings.Split(p, "/")
	if len(segs) > 0 && segs[0] == "" {
		segs = segs[1:]
	}

	return segs
}

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

func isEthDown(f net.Flags) bool {
	return f&net.FlagUp != net.FlagUp
}

func isLoopback(f net.Flags) bool {
	return f&net.FlagLoopback == net.FlagLoopback
}

func getInstanceId() string {
	uuidV7, err := uuid.NewV7()
	if err != nil {
		return uuid.New().String()
	}
	return uuidV7.String()
}
