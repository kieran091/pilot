package pilot

import (
	"path/filepath"
	"testing"
)

func TestCompileProto_FileDescriptorSetIncludesTransitiveImports(t *testing.T) {
	sr := NewServiceRegistrar("User", ":0", nil)
	sr.files = append(sr.files, filepath.Join("example", "apps", "user", "user.proto"))
	sr.protoPaths = append(sr.protoPaths,
		filepath.Join("example", "apps", "user"),
		filepath.Join("example", "third_party"),
		"example",
		".",
	)

	fds, err := sr.compileProto()
	if err != nil {
		t.Fatalf("compileProto failed: %v", err)
	}

	if _, err := fileDescriptorSet2FileDescriptor(fds); err != nil {
		t.Fatalf("fileDescriptorSet2FileDescriptor failed: %v", err)
	}
}
