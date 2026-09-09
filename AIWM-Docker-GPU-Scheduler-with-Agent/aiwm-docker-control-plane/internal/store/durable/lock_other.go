//go:build !linux && !windows

package durable

import (
	"fmt"
	"os"
)

func lockFile(_ *os.File) error { return fmt.Errorf("durable store supports Linux and Windows") }
