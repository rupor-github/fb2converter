//go:build !windows

package config

import (
	"os"

	"golang.org/x/term"
)

// EnableColorOutput checks if colorized output is possible.
func EnableColorOutput(stream *os.File) bool {
	return term.IsTerminal(int(stream.Fd()))
}
