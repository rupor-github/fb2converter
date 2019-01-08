// +build !windows

package config

import (
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

// EnableColorOutput checks if colorized output is possible.
func EnableColorOutput(stream *os.File) bool {
	return terminal.IsTerminal(int(stream.Fd()))
}
