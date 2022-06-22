// Package commands has top level command drivers.
package commands

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"

	"fb2converter/archive"
	"fb2converter/config"
	"fb2converter/processor"
	"fb2converter/state"
)

// processBook processes single FB2 file. "src" is part of the source path (always including file name) relative to the original
// path. When actual file was specified it will be just base file name without a path. When looking inside archive or directory
// it will be relative path inside archive or directory (including base file name).
func processBook(r io.Reader, enc srcEncoding, src, dst string, nodirs, stk, overwrite bool, format processor.OutputFmt, env *state.LocalEnv) error {

	var fname, id string

	env.Log.Info("Conversion starting", zap.String("from", src))
	defer func(start time.Time) {
		if r := recover(); r != nil {
			env.Log.Error("Conversion ended with panic", zap.Any("panic", r), zap.Duration("elapsed", time.Since(start)), zap.String("to", fname), zap.ByteString("stack", debug.Stack()))
		} else {
			env.Log.Info("Conversion completed", zap.Duration("elapsed", time.Since(start)), zap.String("to", fname), zap.String("red_id", id))
		}
	}(time.Now())

	p, err := processor.NewFB2(selectReader(r, enc), enc == encUnknown, src, dst, nodirs, stk, overwrite, format, env)
	if err != nil {
		return err
	}
	id = p.Book.ID.String() // store for reference in the log

	if err = p.Process(); err != nil {
		return err
	}
	if fname, err = p.Save(); err != nil {
		return err
	}

	// store convertion result
	env.Rpt.Store(fmt.Sprintf("fb2c-%s/%s", id, filepath.Base(fname)), fname)

	if err = p.SendToKindle(fname); err != nil {
		return err
	}
	return p.Clean()
}

// processDir walks directory tree finding fb2 files and processes them.
func processDir(dir string, format processor.OutputFmt, nodirs, stk, overwrite bool, cpage encoding.Encoding, dst string, env *state.LocalEnv) (err error) {

	count := 0
	defer func() {
		if err == nil && count == 0 {
			env.Log.Debug("Nothing to process", zap.String("dir", dir))
		}
	}()

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			env.Log.Warn("Skipping path", zap.String("path", path), zap.Error(err))
		} else if info.Mode().IsRegular() {
			var enc srcEncoding
			if ok, err := isArchiveFile(path); err != nil {
				// checking format - but cannot open target file
				env.Log.Warn("Skipping file", zap.String("file", path), zap.Error(err))
			} else if ok {
				if err := processArchive(path, "", filepath.Dir(strings.TrimPrefix(path, dir)), format, nodirs, stk, overwrite, cpage, dst, env); err != nil {
					env.Log.Error("Unable to process archive", zap.String("file", path), zap.Error(err))
				}
			} else if ok, enc, err = isBookFile(path); err != nil {
				env.Log.Warn("Skipping file", zap.String("file", path), zap.Error(err))
			} else if ok {
				count++
				// encoding will be handled properly by processBook
				if file, err := os.Open(path); err != nil {
					env.Log.Error("Unable to process file", zap.String("file", path), zap.Error(err))
				} else {
					defer file.Close()
					if err := processBook(file, enc,
						strings.TrimPrefix(strings.TrimPrefix(path, dir), string(filepath.Separator)), dst,
						nodirs, stk, overwrite, format, env); err != nil {

						env.Log.Error("Unable to process file", zap.String("file", path), zap.Error(err))
					}
				}
			} else {
				env.Log.Debug("Skipping file, not recognized as book or archive", zap.String("file", path))
			}
		}
		return nil
	})
	return err
}

// processArchive walks all files inside archive, finds fb2 files under "pathIn" and processes them.
func processArchive(path, pathIn, pathOut string, format processor.OutputFmt, nodirs, stk, overwrite bool, cpage encoding.Encoding, dst string, env *state.LocalEnv) (err error) {

	count := 0
	defer func() {
		if err == nil && count == 0 {
			env.Log.Debug("Nothing to process", zap.String("archive", path))
		}
	}()

	err = archive.Walk(path, pathIn, func(archive string, f *zip.File) error {
		if ok, enc, err := isBookInArchive(f); err != nil {
			env.Log.Warn("Skipping file in archive",
				zap.String("archive", archive),
				zap.String("path", f.FileHeader.Name),
				zap.Error(err))
		} else if ok {
			count++
			// encoding will be handled properly by processBook
			if r, err := f.Open(); err != nil {
				env.Log.Error("Unable to process file in archive",
					zap.String("archive", archive),
					zap.String("file", f.FileHeader.Name),
					zap.Error(err))
			} else {
				defer r.Close()
				apath := f.FileHeader.Name
				if cpage != nil && f.FileHeader.NonUTF8 {
					// forcing zip file name encoding
					if n, err := cpage.NewDecoder().String(apath); err == nil {
						apath = n
					} else {
						n, _ = ianaindex.IANA.Name(cpage)
						env.Log.Warn("Unable to convert archive name from specified encoding", zap.String("charset", n), zap.String("path", apath), zap.Error(err))
					}
				}
				if err := processBook(r, enc, filepath.Join(pathOut, apath), dst, nodirs, stk, overwrite, format, env); err != nil {
					env.Log.Error("Unable to process file in archive",
						zap.String("archive", archive),
						zap.String("file", f.FileHeader.Name),
						zap.Error(err))
				}
			}
		} else {
			env.Log.Debug("Skipping file, not recognized as book", zap.String("archive", archive), zap.String("file", f.FileHeader.Name))
		}
		return nil
	})
	return err
}

// Convert is "convert" command body.
func Convert(ctx *cli.Context) (err error) {

	const (
		errPrefix = "convert: "
		errCode   = 1
	)

	env := ctx.Generic(state.FlagName).(*state.LocalEnv)

	src := ctx.Args().Get(0)
	if len(src) == 0 {
		return cli.Exit(errors.New(errPrefix+"no input source has been specified"), errCode)
	}
	src, err = filepath.Abs(src)
	if err != nil {
		return cli.Exit(fmt.Errorf("%snormalizing source path failed", errPrefix), errCode)
	}

	dst := ctx.Args().Get(1)
	if len(dst) == 0 {
		if dst, err = os.Getwd(); err != nil {
			return cli.Exit(fmt.Errorf("%sunable to get working directory", errPrefix), errCode)
		}
	} else {
		if dst, err = filepath.Abs(dst); err != nil {
			return cli.Exit(fmt.Errorf("%snormalizing destination path failed", errPrefix), errCode)
		}
		if ctx.Args().Len() > 2 {
			env.Log.Warn("Mailformed command line, too many destinations", zap.Strings("ignoring", ctx.Args().Slice()[2:]))
		}
	}

	var format processor.OutputFmt
	switch env.Mhl {
	case config.MhlMobi:
		format = processor.ParseFmtString(env.Cfg.Fb2Mobi.OutputFormat)
		if format == processor.UnsupportedOutputFmt || format == processor.OEpub || format == processor.OKepub {
			env.Log.Warn("Unknown output format in MHL mode requested, switching to mobi", zap.String("format", env.Cfg.Fb2Mobi.OutputFormat))
			format = processor.OMobi
		}
	case config.MhlEpub:
		format = processor.ParseFmtString(env.Cfg.Fb2Epub.OutputFormat)
		if format == processor.UnsupportedOutputFmt || format == processor.OMobi || format == processor.OAzw3 {
			env.Log.Warn("Unknown output format in MHL mode requested, switching to epub", zap.String("format", env.Cfg.Fb2Epub.OutputFormat))
			format = processor.OEpub
		}
	default:
		format = processor.ParseFmtString(ctx.String("to"))
		if format == processor.UnsupportedOutputFmt {
			env.Log.Warn("Unknown output format requested, switching to epub", zap.String("format", ctx.String("to")))
			format = processor.OEpub
		}
	}
	nodirs := ctx.Bool("nodirs")
	overwrite := ctx.Bool("ow")

	if !env.Cfg.Doc.ChapterPerFile && (env.Cfg.Doc.PagesPerFile != math.MaxInt32 || len(env.Cfg.Doc.ChapterDividers) > 0) {
		env.Log.Warn("With chapter_per_file=false settings to control resulting content size (ex: pages_per_file, chapter_subtitle_dividers) will be ignored")
	}

	var cpage encoding.Encoding

	page := ctx.String("force-zip-cp")
	if len(page) > 0 {
		cpage, err = ianaindex.IANA.Encoding(page)
		if err != nil {
			env.Log.Warn("Unknown character set specification. Ignoring...", zap.String("charset", page), zap.Error(err))
			cpage = nil
		} else {
			n, _ := ianaindex.IANA.Name(cpage)
			env.Log.Debug("Forcefully convert all non UTF-8 file names in archives", zap.String("charset", n))
		}
	}

	stk := ctx.Bool("stk")
	if env.Mhl == config.MhlEpub {
		stk = env.Cfg.Fb2Epub.SendToKindle
	}
	if stk && format != processor.OEpub {
		env.Log.Warn("Send to Kindle could only be used with epub output format, turning off", zap.Stringer("format", format))
		stk = false
	}

	env.Log.Info("Processing starting", zap.String("source", src), zap.String("destination", dst), zap.Stringer("format", format))
	defer func(start time.Time) {
		env.Log.Info("Processing completed", zap.Duration("elapsed", time.Since(start)))
	}(time.Now())

	var head, tail string
	for head = src; len(head) != 0; head, tail = filepath.Split(head) {

		head = strings.TrimSuffix(head, string(filepath.Separator))

		fi, err := os.Stat(head)
		if err != nil {
			// does not exists - probably path in archive
			continue
		}

		if fi.Mode().IsDir() {
			if len(tail) != 0 {
				// directory cannot have tail - it would be simple file
				return cli.Exit(fmt.Errorf("%sinput source was not found (%s) => (%s)", errPrefix, head, strings.TrimPrefix(src, head)), errCode)
			}
			if err := processDir(head, format, nodirs, stk, overwrite, cpage, dst, env); err != nil {
				return cli.Exit(fmt.Errorf("%sunable to process directory", errPrefix), errCode)
			}
			break
		}

		if fi.Mode().IsRegular() {

			ok, err := isArchiveFile(head)
			if err != nil {
				// checking format - but cannot open target file
				return cli.Exit(fmt.Errorf("%sunable to check archive type: %w", errPrefix, err), errCode)
			}

			if ok {
				// we need to look inside to see if path makes sense
				tail = strings.TrimPrefix(strings.TrimPrefix(src, head), string(filepath.Separator))
				if err := processArchive(head, tail, "", format, nodirs, stk, overwrite, cpage, dst, env); err != nil {
					return cli.Exit(fmt.Errorf("%sunable to process archive: %w", errPrefix, err), errCode)
				}
				break
			}

			var enc srcEncoding
			ok, enc, err = isBookFile(head)
			if err != nil {
				// checking format - but cannot open target file
				return cli.Exit(fmt.Errorf("%sunable to check file type: %w", errPrefix, err), errCode)

			}

			if ok && len(tail) == 0 {
				// we have book, it cannot have tail
				// encoding will be handled properly by processBook
				if file, err := os.Open(head); err != nil {
					env.Log.Error("Unable to process file", zap.String("file", head), zap.Error(err))
				} else {
					defer file.Close()
					if err := processBook(file, enc, filepath.Base(head), dst, nodirs, stk, overwrite, format, env); err != nil {
						env.Log.Error("Unable to process file", zap.String("file", head), zap.Error(err))
					}
				}
				break
			}

			return cli.Exit(fmt.Errorf("%sinput was not recognized as FB2 book (%s)", errPrefix, head), errCode)
		}

		return cli.Exit(fmt.Errorf("%sunexpected path mode for (%s) => (%s)", errPrefix, head, strings.TrimPrefix(src, head)), errCode)
	}
	if len(head) == 0 {
		return cli.Exit(fmt.Errorf("%sinput source was not found (%s)", errPrefix, src), errCode)
	}

	return nil
}
