//go:build !windows

package configstore

import (
	"errors"
	"os"
	"syscall"
)

func checkOwner(info os.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() || int(st.Gid) != os.Getegid() {
		return errors.New("configuration_owner_mismatch")
	}
	return nil
}
