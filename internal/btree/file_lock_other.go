//go:build !linux && !darwin

package btree

import (
	"fmt"
	"os"
)

func lockDatabaseFile(file *os.File) error {
	return fmt.Errorf("exclusive database-file ownership is unsupported on this platform: %s", file.Name())
}
