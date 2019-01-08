// +build darwin

package config

import (
	"os"
	"strings"
)

// kindlegen provides OS specific part of default kindlegen location
func kindlegen() string {
	return "kindlegen"
}

var toRemove = "." + string(os.PathSeparator) + string(os.PathListSeparator)

// CleanFileName removes not allowed characters form file name.
func CleanFileName(in string) string {
	return strings.Map(func(sym rune) rune {
		if strings.IndexRune(toRemove, sym) != -1 {
			return -1
		}
		return sym
	}, in)
}

// FindConverter  - used on Windows to support myhomelib
func FindConverter(_ string) string {
	return ""
}
