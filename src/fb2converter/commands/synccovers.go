package commands

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"go.uber.org/zap"

	"fb2converter/processor"
	"fb2converter/state"
)

// SyncCovers reads books in Kindle formats and produces thumbnails for them. Very Kindle specific.
func SyncCovers(ctx *cli.Context) error {

	// var err error

	const (
		errPrefix = "\n*** ERROR ***\n\nsynccovers: "
		errCode   = 1
	)

	env := ctx.GlobalGeneric(state.FlagName).(*state.LocalEnv)

	indir := ctx.Args().Get(0)
	if len(indir) == 0 {
		return cli.NewExitError(errors.New(errPrefix+"books directory has not been specified"), errCode)
	}

	width := ctx.Int("width")
	height := ctx.Int("height")
	stretch := ctx.Bool("stretch")

	var dir string
	// let's see if we could locate kindle directory
	for head, tail := filepath.Split(strings.TrimSuffix(indir, string(os.PathSeparator))); len(tail) > 0; head, tail = filepath.Split(strings.TrimSuffix(head, string(os.PathSeparator))) {
		dir = filepath.Join(head, "system", "thumbnails")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
	}
	if len(dir) == 0 {
		return cli.NewExitError(errors.New(errPrefix+"unable to find Kindle system directory along the specified path"), errCode)
	}

	files, count := 0, 0

	start := time.Now()
	env.Log.Info("Thumbnail extraction starting", zap.String("kindle directory", dir))
	defer func(start time.Time) {
		env.Log.Info("Thumbnail extraction completed", zap.Duration("elapsed", time.Now().Sub(start)), zap.Int("files", files), zap.Int("extracted", count))
	}(start)

	err := filepath.Walk(indir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		} else if info.Mode().IsRegular() {
			ext := filepath.Ext(info.Name())
			if strings.EqualFold(ext, ".mobi") || strings.EqualFold(ext, ".azw3") {
				env.Log.Debug("Creating thumbnail", zap.String("file", path))
				files++
				created, err := processor.ProduceThumbnail(path, dir, width, height, stretch, env.Log)
				if err != nil {
					return err
				}
				if created {
					count++
				}
			}
		}
		return nil
	})

	if err != nil {
		env.Log.Error("Unable to process Kindle files", zap.Error(err))
	}
	return nil
}
