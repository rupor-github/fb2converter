package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli"
	"go.uber.org/zap"

	"fb2converter/processor"
	"fb2converter/state"
)

// processEpub processes single EPUB file. "src" is part of the source path (always including file name) relative to the original
// path. When actual file was specified it will be just base file name without a path. When looking inside archive or directory
// it will be relative path inside archive or directory (including base file name).
func processEpub(r io.Reader, src, dst string, nodirs, stk, overwrite bool, format processor.OutputFmt, env *state.LocalEnv) error {

	var fname string

	env.Log.Info("Transfer starting", zap.String("from", src))
	defer func(start time.Time) {
		env.Log.Info("Transfer completed", zap.Duration("elapsed", time.Since(start)), zap.String("to", fname))
	}(time.Now())

	p, err := processor.NewEPUB(r, src, dst, nodirs, stk, overwrite, format, env)
	if err != nil {
		return err
	}
	if err = p.Process(); err != nil {
		return err
	}
	if fname, err = p.Save(); err != nil {
		return err
	}
	if err = p.SendToKindle(fname); err != nil {
		return err
	}
	return p.Clean()
}

// Transfer is "transfer" command body.
func Transfer(ctx *cli.Context) (err error) {

	const (
		errPrefix = "transfer: "
		errCode   = 1
	)

	env := ctx.GlobalGeneric(state.FlagName).(*state.LocalEnv)

	src := ctx.Args().Get(0)
	if len(src) == 0 {
		return cli.NewExitError(errors.New(errPrefix+"no input source has been specified"), errCode)
	}
	src, err = filepath.Abs(src)
	if err != nil {
		return cli.NewExitError(fmt.Errorf("%scleaning source path failed: %w", errPrefix, err), errCode)
	}

	dst := ctx.Args().Get(1)
	if len(dst) == 0 {
		if dst, err = os.Getwd(); err != nil {
			return cli.NewExitError(fmt.Errorf("%sunable to get working directory: %w", errPrefix, err), errCode)
		}
	} else {
		if dst, err = filepath.Abs(dst); err != nil {
			return cli.NewExitError(fmt.Errorf("%scleaning destination path failed: %w", errPrefix, err), errCode)
		}
	}

	format := processor.ParseFmtString(ctx.String("to"))
	if format == processor.UnsupportedOutputFmt || (format != processor.OMobi && format != processor.OAzw3) {
		env.Log.Warn("Unknown output format requested, switching to mobi", zap.String("format", ctx.String("to")))
		format = processor.OMobi
	}

	nodirs := ctx.Bool("nodirs")
	overwrite := ctx.Bool("ow")

	stk := ctx.Bool("stk")
	if stk && format != processor.OMobi {
		env.Log.Warn("Send to Kindle could only be used with mobi output format, turning off", zap.Stringer("format", format))
		stk = false
	}

	env.Log.Info("Processing starting", zap.String("source", src), zap.String("destination", dst), zap.Stringer("format", format))
	defer func(start time.Time) {
		env.Log.Info("Processing completed", zap.Duration("elapsed", time.Since(start)))
	}(time.Now())

	fi, err := os.Stat(src)
	if err != nil {
		return cli.NewExitError(fmt.Errorf("%sinput source was not found (%s)", errPrefix, src), errCode)
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		count := 0
		if err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				env.Log.Warn("Skipping path", zap.String("path", path), zap.Error(err))
			} else if info.Mode().IsRegular() {
				if ok, err := isEpubFile(path); err != nil {
					// checking format - but cannot open target file
					env.Log.Warn("Skipping file", zap.String("file", path), zap.Error(err))
				} else if ok {
					count++
					if file, err := os.Open(path); err != nil {
						env.Log.Error("Unable to process file", zap.String("file", path), zap.Error(err))
					} else {
						defer file.Close()
						if err := processEpub(file,
							strings.TrimPrefix(strings.TrimPrefix(path, src), string(filepath.Separator)), dst,
							nodirs, stk, overwrite, format, env); err != nil {

							env.Log.Error("Unable to process file", zap.String("file", path), zap.Error(err))
						}
					}
				}
			}
			return nil
		}); err != nil {
			return cli.NewExitError(fmt.Errorf("%sunable to process directory: %w", errPrefix, err), errCode)
		}
		if count == 0 {
			env.Log.Debug("Nothing to process", zap.String("dir", src))
		}
	case mode.IsRegular():
		if ok, err := isEpubFile(src); err != nil {
			// checking format - but cannot open target file
			return cli.NewExitError(fmt.Errorf("%sunable to check file type: %w", errPrefix, err), errCode)
		} else if !ok {
			// wrong file type
			return cli.NewExitError(fmt.Errorf("%sinput was not recognized as epub book (%s)", errPrefix, src), errCode)
		}
		if file, err := os.Open(src); err != nil {
			env.Log.Error("Unable to process file", zap.String("file", src), zap.Error(err))
		} else {
			defer file.Close()
			if err := processEpub(file, filepath.Base(src), dst, nodirs, stk, overwrite, format, env); err != nil {
				env.Log.Error("Unable to process file", zap.String("file", src), zap.Error(err))
			}
		}
	default:
		return cli.NewExitError(fmt.Errorf("%sunsupported type of input source (%s)", errPrefix, src), errCode)
	}
	return nil
}
