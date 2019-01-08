package processor

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// FinalizeEPUB produces epub file out of previously saved temporary files.
func (p *Processor) FinalizeEPUB(fname string) error {

	if _, err := os.Stat(fname); err == nil {
		if !p.env.Debug {
			return errors.Errorf("output file already exists: %s", fname)
		}
		// NOTE: when debugging - ignore existing file
		p.env.Log.Debug("Overwriting existing file - debug mode", zap.String("file", fname))
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

	f, err := os.Create(fname)
	if err != nil {
		return errors.Wrapf(err, "unable to create EPUB: %s", fname)
	}
	defer f.Close()

	epub := zip.NewWriter(f)
	defer epub.Close()

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

		// Get the path of the file relative to the source folder
		rel, err := filepath.Rel(p.tmpDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		var w io.Writer
		if info.Name() == "mimetype" {
			if w, err = epub.CreateHeader(&zip.FileHeader{
				Name:   "mimetype",
				Method: zip.Store,
			}); err != nil {
				return err
			}
		} else {
			if w, err = epub.Create(rel); err != nil {
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

	if err = filepath.Walk(p.tmpDir, saveFile); err != nil {
		return errors.Wrap(err, "unable to add file to EPUB")
	}
	return nil
}
