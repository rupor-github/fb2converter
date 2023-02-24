//go:build tools
// +build tools

package tools

import (
	//  To keep go mod happy
	_ "golang.org/x/tools/cmd/stringer"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
