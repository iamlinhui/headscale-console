//go:build windows

package configstore

import (
	"os"
)

// Windows does not expose POSIX ownership; deployment ACLs must grant both services read access.
func checkOwner(info os.FileInfo) error { return nil }
