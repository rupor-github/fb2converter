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

	const (
		errPrefix = "synccovers: "
		errCode   = 1
	)

	env := ctx.Generic(state.FlagName).(*state.LocalEnv)

	if ctx.Args().Len() > 2 {
		env.Log.Warn("Mailformed command line", zap.Strings("ignoring", ctx.Args().Slice()[1:]))
	}

	src := ctx.Args().Get(0)
	if len(src) == 0 {
		return cli.Exit(errors.New(errPrefix+"book source has not been specified"), errCode)
	}

	in, err := filepath.Abs(src)
	if err != nil {
		return cli.Exit(fmt.Errorf("%snormalizing book source path failed: %w", errPrefix, err), errCode)
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

	var dst string
	if ctx.Args().Len() == 1 {
		// let's see if we could locate kindle directory
		for head, tail := filepath.Split(strings.TrimSuffix(dir, string(os.PathSeparator))); len(tail) > 0; head, tail = filepath.Split(strings.TrimSuffix(head, string(os.PathSeparator))) {
			dst = filepath.Join(head, "system", "thumbnails")
			if info, err := os.Stat(dst); err == nil && info.IsDir() {
				break
			}
		}
		info, err := os.Stat(dst)
		if os.IsNotExist(err) {
			return cli.Exit(fmt.Errorf("%sunable to find Kindle thumbnails directory along the specified path", errPrefix), errCode)
		} else if err != nil {
			return cli.Exit(fmt.Errorf("%swrong Kindle source path has been specified: %w", errPrefix, err), errCode)
		}
		if !info.IsDir() {
			return cli.Exit(fmt.Errorf("%sthumbnails path must be a directory", errPrefix), errCode)
		}
	} else {
		// ignore kindle directory logic
		dst = ctx.Args().Get(1)

		if len(dst) == 0 {
			return cli.Exit(fmt.Errorf("%sempty destination path has been specified", errPrefix), errCode)
		}
		if dst, err = filepath.Abs(dst); err != nil {
			return cli.Exit(fmt.Errorf("%snormalizing destination path failed", errPrefix), errCode)
		}
		info, err := os.Stat(dst)
		if os.IsNotExist(err) {
			return cli.Exit(fmt.Errorf("%sdestination path must exist", errPrefix), errCode)
		} else if err != nil {
			return cli.Exit(fmt.Errorf("%swrong destination path has been specified: %w", errPrefix, err), errCode)
		}
		if !info.IsDir() {
			return cli.Exit(fmt.Errorf("%sdestination path must be a directory", errPrefix), errCode)
		}
	}

	files, count := 0, 0

	makeThumb := func(file, path string) error {
		ext := filepath.Ext(file)
		if strings.EqualFold(ext, ".mobi") || strings.EqualFold(ext, ".azw3") {
			env.Log.Debug("Creating thumbnail", zap.String("file", path))
			files++
			created, err := processor.ProduceThumbnail(path, dst, width, height, stretch, env.Log)
			if err != nil {
				return err
			}
			if created {
				count++
			}
		}
		return nil
	}

	env.Log.Info("Thumbnail extraction starting", zap.String("kindle directory", dst))
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
