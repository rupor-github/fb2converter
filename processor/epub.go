package processor

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"time"

	fixzip "github.com/hidez8891/zip"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func zipRemoveDataDescriptors(from, to string) error {

	out, err := os.Create(to)
	if err != nil {
		return errors.Wrapf(err, "unable to create EPUB: %s", to)
	}

	r, err := fixzip.OpenReader(from)
	if err != nil {
		return errors.Wrapf(err, "unable to read EPUB: %s", from)
	}
	defer r.Close()

	w := fixzip.NewWriter(out)
	defer w.Close()

	for _, file := range r.File {
		// unset data descriptor flag.
		file.Flags &= ^fixzip.FlagDataDescriptor

		// copy zip entry
		w.CopyFile(file)
	}
	return nil
}

func (p *Processor) writeEPUB(fname string) error {

	f, err := os.Create(fname)
	if err != nil {
		return errors.Wrapf(err, "unable to create EPUB: %s", fname)
	}
	defer f.Close()

	epub := zip.NewWriter(f)
	defer epub.Close()

	var content bool
	// fd, ft := timeToMsDosTime(time.Now())
	t := time.Now()

	saveFile := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if filepath.ToSlash(path) == filepath.ToSlash(fname) {
			// ignore itself
			return nil
		}
		if content && filepath.ToSlash(filepath.Dir(path)) == filepath.ToSlash(p.tmpDir) {
			// ignore everything in the root directory
			return nil
		}

		// Get the path of the file relative to the source folder
		rel, err := filepath.Rel(p.tmpDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		var w io.Writer
		if !content {
			if w, err = epub.CreateHeader(&zip.FileHeader{
				Name:     info.Name(),
				Method:   zip.Store,
				Modified: t,
			}); err != nil {
				return err
			}
		} else {
			if w, err = epub.CreateHeader(&zip.FileHeader{
				Name:     rel,
				Method:   zip.Deflate,
				Modified: t,
			}); err != nil {
				return err
			}
		}

		var r io.ReadCloser
		if r, err = os.Open(path); err != nil {
			return err
		}
		defer r.Close()

		if _, err = io.Copy(w, r); err != nil {
			return err
		}
		return nil
	}

	// mimetype should be the first entry in epub
	mt := filepath.Join(p.tmpDir, "mimetype")
	info, err := os.Stat(mt)
	if err != nil {
		return errors.Wrap(err, "unable to find mimetype file")
	}
	if err = saveFile(mt, info, nil); err != nil {
		return errors.Wrap(err, "unable to add mimetype to EPUB")
	}

	content = true

	if err = filepath.Walk(p.tmpDir, saveFile); err != nil {
		return errors.Wrap(err, "unable to add file to EPUB")
	}
	return nil
}

// FinalizeEPUB produces epub file out of previously saved temporary files.
func (p *Processor) FinalizeEPUB(fname string) error {

	if _, err := os.Stat(fname); err == nil {
		if !p.env.Debug && !p.overwrite {
			return errors.Errorf("output file already exists: %s", fname)
		}
		p.env.Log.Warn("Overwriting existing file", zap.String("file", fname))
		if err = os.Remove(fname); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		if err := os.MkdirAll(filepath.Dir(fname), 0700); err != nil {
			return errors.Wrap(err, "unable to create output directory")
		}
	}

	if p.env.Cfg.Doc.FixZip {
		_, tmp := filepath.Split(fname)
		tmp = filepath.Join(p.tmpDir, tmp)

		if err := p.writeEPUB(tmp); err != nil {
			return err
		}
		if p.env.Cfg.Doc.FixZip {
			return zipRemoveDataDescriptors(tmp, fname)
		}
	} else {
		if err := p.writeEPUB(fname); err != nil {
			return err
		}
	}
	return nil
}

// FinalizeKEPUB produces kepub.epub file out of previously saved temporary files.
func (p *Processor) FinalizeKEPUB(fname string) error {
	return p.FinalizeEPUB(fname)
}
