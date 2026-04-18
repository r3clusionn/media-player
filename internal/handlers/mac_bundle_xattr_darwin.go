//go:build darwin

package handlers

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const macQuarantineAttribute = "com.apple.quarantine"

func clearMacAppQuarantine(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		var err error
		if d.Type()&os.ModeSymlink != 0 {
			err = unix.Lremovexattr(path, macQuarantineAttribute)
		} else {
			err = unix.Removexattr(path, macQuarantineAttribute)
		}

		if err != nil && !errors.Is(err, unix.ENOATTR) {
			return err
		}

		return nil
	})
}
