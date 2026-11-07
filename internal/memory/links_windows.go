//go:build windows

package memory

import "os"

// hardLinked is always false on Windows (native Windows is out of scope).
func hardLinked(os.FileInfo) bool { return false }
