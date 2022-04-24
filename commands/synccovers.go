package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"fb2converter/processor"
	"fb2converter/state"
)

// SyncCovers reads books in Kindle formats and produces thumbnails for them. Very Kindle specific.
func SyncCovers(ctx *cli.Context) error {

	// var err error

	const (
		errPrefix = "synccovers: "
		errCode   = 1
	)

	env := ctx.Generic(state.FlagName).(*state.LocalEnv)

	if len(ctx.Args().Get(0)) == 0 {
		return cli.Exit(errors.New(errPrefix+"book source has not been specified"), errCode)
	}

	in, err := filepath.Abs(ctx.Args().Get(0))
	if err != nil {
		return cli.Exit(fmt.Errorf("%swrong book source has been specified: %w", errPrefix, err), errCode)
	}

	dir, file := in, ""
	if info, err := os.Stat(in); err != nil {
		return cli.Exit(fmt.Errorf("%swrong book source has been specified: %w", errPrefix, err), errCode)
	} else if info.Mode().IsRegular() {
		dir, file = filepath.Split(in)
	}

	width := ctx.Int("width")
	height := ctx.Int("height")
	stretch := ctx.Bool("stretch")

	var sysdir string
	// let's see if we could locate kindle directory
	for head, tail := filepath.Split(strings.TrimSuffix(dir, string(os.PathSeparator))); len(tail) > 0; head, tail = filepath.Split(strings.TrimSuffix(head, string(os.PathSeparator))) {
		sysdir = filepath.Join(head, "system", "thumbnails")
		if info, err := os.Stat(sysdir); err == nil && info.IsDir() {
			break
		}
	}
	if len(sysdir) == 0 {
		return cli.Exit(errors.New(errPrefix+"unable to find Kindle system directory along the specified path"), errCode)
	}

	files, count := 0, 0

	makeThumb := func(file, path string) error {
		ext := filepath.Ext(file)
		if strings.EqualFold(ext, ".mobi") || strings.EqualFold(ext, ".azw3") {
			env.Log.Debug("Creating thumbnail", zap.String("file", path))
			files++
			created, err := processor.ProduceThumbnail(path, sysdir, width, height, stretch, env.Log)
			if err != nil {
				return err
			}
			if created {
				count++
			}
		}
		return nil
	}

	env.Log.Info("Thumbnail extraction starting", zap.String("kindle directory", sysdir))
	defer func(start time.Time) {
		env.Log.Info("Thumbnail extraction completed", zap.Duration("elapsed", time.Since(start)), zap.Int("files", files), zap.Int("extracted", count))
	}(time.Now())

	if len(file) > 0 {
		// single file
		err = makeThumb(file, in)
	} else {
		// directory
		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			} else if info.Mode().IsRegular() {
				return makeThumb(info.Name(), path)
			}
			return nil
		})
	}

	if err != nil {
		env.Log.Error("Unable to process Kindle files", zap.Error(err))
	}
	return nil
}
