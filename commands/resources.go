package commands

import (
	"errors"
	"os"

	"github.com/urfave/cli/v2"

	"fb2converter/processor"
	"fb2converter/state"
	"fb2converter/static"
)

// ExportResources is "export" command body.
func ExportResources(ctx *cli.Context) error {

	// var err error

	const (
		errPrefix = "export: "
		errCode   = 1
	)

	env := ctx.Generic(state.FlagName).(*state.LocalEnv)

	fname := ctx.Args().Get(0)
	if len(fname) == 0 {
		return cli.Exit(errors.New(errPrefix+"destination directory has not been specified"), errCode)
	}
	if info, err := os.Stat(fname); err != nil && !os.IsNotExist(err) {
		return cli.Exit(errors.New(errPrefix+"unable to access destination directory"), errCode)
	} else if err != nil {
		return cli.Exit(errors.New(errPrefix+"destination directory does not exits"), errCode)
	} else if !info.IsDir() {
		return cli.Exit(errors.New(errPrefix+"destination is not a directory"), errCode)
	}

	ignoreNames := map[string]bool{
		processor.DirHyphenator: true,
		processor.DirResources:  true,
		processor.DirSentences:  true,
	}

	if dir, err := static.AssetDir(""); err == nil {
		for _, a := range dir {
			if _, ignore := ignoreNames[a]; env.Debug || !ignore {
				err = static.RestoreAssets(fname, a)
				if err != nil {
					return cli.Exit(errors.New(errPrefix+"unable to store resources"), errCode)
				}
			}
		}
	}
	return nil
}
