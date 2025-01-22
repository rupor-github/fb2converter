//go:build linux || freebsd

package config

import (
	"os"
	"strings"
)

// kindlegen provides OS specific part of default kindlegen location
func kindlegen() string {
	return "kindlegen"
}

// CleanFileName removes not allowed characters form file name.
func CleanFileName(in string) string {
	out := strings.TrimLeft(strings.Map(func(sym rune) rune {
		if strings.ContainsRune(string(os.PathSeparator)+string(os.PathListSeparator), sym) {
			return -1
		}
		return sym
	}, in), ".")
	if len(out) == 0 {
		out = "_bad_file_name_"
	}
	return out
}

// FindConverter  - used on Windows to support myhomelib
func FindConverter(_ string) string {
	return ""
}
