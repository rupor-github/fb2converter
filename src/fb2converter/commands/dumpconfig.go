package commands

import (
	"os"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"go.uber.org/zap"

	"fb2converter/state"
)

// DumpConfig is "dumpconfig" command body.
func DumpConfig(ctx *cli.Context) error {

	var err error

	const (
		errPrefix = "\n*** ERROR ***\n\ndumpconfig: "
		errCode   = 1
	)

	env := ctx.GlobalGeneric(state.FlagName).(*state.LocalEnv)

	fname := ctx.Args().Get(0)

	out := os.Stdout
	if len(fname) > 0 {
		out, err = os.Create(fname)
		if err != nil {
			return cli.NewExitError(errors.New(errPrefix+"unable to use destination file"), errCode)
		}
		defer out.Close()

		env.Log.Info("Dumping configuration", zap.String("file", fname))
	}

	var data []byte
	if env.Debug {
		data, err = env.Cfg.GetBytes()
	} else {
		data, err = env.Cfg.GetActualBytes()
	}
	if err != nil {
		return cli.NewExitError(errors.Wrapf(err, "%sunable to get configuration", errPrefix), errCode)
	}

	_, err = out.Write(data)
	if err != nil {
		return cli.NewExitError(errors.Wrapf(err, "%sunable to write configuration", errPrefix), errCode)
	}
	return nil
}
