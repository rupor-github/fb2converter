package processor

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/rupor-github/fb2converter/etree"
)

type dataTransientFlags uint8

const (
	dataNotForManifest dataTransientFlags = 1 << iota
	dataNotForSpline
)

// Any file which needs processing/saving, for example css.
type dataFile struct {
	id        string
	fname     string
	nofmt     bool
	relpath   string             // always relative to "root" directory - usually temporary working directory
	transient dataTransientFlags // Additional information about file placements
	ct        string
	data      []byte
	doc       *etree.Document
}

func (f *dataFile) flush(path string) error {

	if len(f.fname) == 0 || (len(f.data) == 0 && f.doc == nil) {
		return nil
	}

	newdir := filepath.Join(path, f.relpath)
	if err := os.MkdirAll(newdir, 0700); err != nil {
		return errors.Wrap(err, "unable to create content directory")
	}

	if f.doc != nil {
		if f.nofmt {
			// on present day kindles when using mobi/azw3 formats and float-new notes mode chardata records interfere with notes formating on device
			f.doc.Indent(etree.NoIndent)
		} else {
			// this is XML - ignore char data
			f.doc.IndentTabs()
		}
		if err := f.doc.WriteToFile(filepath.Join(newdir, f.fname)); err != nil {
			return errors.Wrapf(err, "unable to flush XML content to %s", filepath.Join(newdir, f.fname))
		}
		return nil
	}

	if err := ioutil.WriteFile(filepath.Join(newdir, f.fname), f.data, 0644); err != nil {
		return errors.Wrapf(err, "unable to save data to (%s)", filepath.Join(newdir, f.fname))
	}
	return nil
}
