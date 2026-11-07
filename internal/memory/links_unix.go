//go:build !windows

package memory

import (
	"os"
	"syscall"
)

// hardLinked reports whether fi has more than one hard link.
func hardLinked(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1
}
