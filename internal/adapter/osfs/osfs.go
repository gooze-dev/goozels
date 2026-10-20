// Package osfs reads files from the local disk. It is the interface adapter
// behind the controller's FileReader port.
package osfs

import "os"

// FS reads files from the operating system's file system.
type FS struct{}

// ReadFile returns the contents of the named file.
func (FS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name) //nolint:gosec // callers name files inside the workspace they check
}
