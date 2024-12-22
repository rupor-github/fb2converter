package commands

import (
	"errors"
	"os"

	cli "github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"fb2converter/processor"
	"fb2converter/state"
	"fb2converter/static"
)

// ExportResources is "export" command body.
func ExportResources(ctx *cli.Context) error {

	const (
		errPrefix = "export: "
		errCode   = 1
	)

	env := ctx.Generic(state.FlagName).(*state.LocalEnv)
	if ctx.Args().Len() > 1 {
		env.Log.Warn("Mailformed command line, too many destinations", zap.Strings("ignoring", ctx.Args().Slice()[1:]))
	}

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
			if _, ignore := ignoreNames[a]; !ignore {
				err = static.RestoreAssets(fname, a)
				if err != nil {
					return cli.Exit(errors.New(errPrefix+"unable to store resources"), errCode)
				}
			}
		}
	}
	return nil
}
