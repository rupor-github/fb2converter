package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"fb2converter/state"
)

// DumpConfig is "dumpconfig" command body.
func DumpConfig(ctx *cli.Context) error {

	var err error

	const (
		errPrefix = "dumpconfig: "
		errCode   = 1
	)

	env := ctx.Generic(state.FlagName).(*state.LocalEnv)
	if ctx.Args().Len() > 1 {
		env.Log.Warn("Mailformed command line, too many destinations", zap.Strings("ignoring", ctx.Args().Slice()[1:]))
	}

	fname := ctx.Args().Get(0)

	out := os.Stdout
	if len(fname) > 0 {
		out, err = os.Create(fname)
		if err != nil {
			return cli.Exit(errors.New(errPrefix+"unable to use destination file"), errCode)
		}
		defer out.Close()

		env.Rpt.Store("dump.json", fname)

		env.Log.Info("Dumping configuration", zap.String("file", fname))
	}

	var data []byte
	data, err = env.Cfg.GetActualBytes()
	if err != nil {
		return cli.Exit(fmt.Errorf("%sunable to get configuration: %w", errPrefix, err), errCode)
	}

	_, err = out.Write(data)
	if err != nil {
		return cli.Exit(fmt.Errorf("%sunable to write configuration: %w", errPrefix, err), errCode)
	}
	return nil
}
