package osfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "x.go")
	if err := os.WriteFile(name, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := (FS{}).ReadFile(name); err != nil || string(got) != "package x\n" {
		t.Errorf("ReadFile = %q, %v", got, err)
	}
	if _, err := (FS{}).ReadFile(name + ".missing"); err == nil {
		t.Error("ReadFile of a missing file should fail")
	}
}
