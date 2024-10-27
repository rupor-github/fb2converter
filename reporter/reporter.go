package reporter

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Reporter accumulates information necessary to prepare debug report.
type Report struct {
	// NOTE: not to be used concurrently!
	paths map[string]string
	file  *os.File
}

// NewReporter() creates initialized empty reporter.
func NewReporter() (*Report, error) {

	r := &Report{paths: make(map[string]string)}

	if f, err := os.Create("fb2c-report.zip"); err == nil {
		r.file = f
	} else if f, err = os.CreateTemp("", "fb2c-report.*.zip"); err == nil {
		r.file = f
	} else {
		return nil, fmt.Errorf("unable to create report: %w", err)
	}
	return r, nil
}

// Close() finalizes debug report.
func (r *Report) Close() error {

	if r.file == nil {
		return nil
	}
	defer r.file.Close()

	return r.finalize()
}

// Name() returns name of underlying file.
func (r *Report) Name() string {

	if r.file == nil {
		return ""
	}
	if n, err := filepath.Abs(r.file.Name()); err == nil {
		return n
	}
	return r.file.Name()
}

// Store() saves path to file or directory to be put in the final archive later.
func (r *Report) Store(name, path string) {

	if r == nil {
		// Ignore uninitialized cases to avoid checking n many places. This means no report has been requested.
		return
	}
	if old, exists := r.paths[name]; exists && old != path {
		// Somewhere I do not know what I am doing.
		panic(fmt.Sprintf("Attempt to overwrite file in the report for [%s]: was %s, now %s", name, old, path))
	}
	// Cleanup the path ignoring errors.
	if p, err := filepath.Abs(path); err == nil {
		r.paths[name] = p
	} else {
		r.paths[name] = path
	}
}

func (r *Report) finalize() error {

	arc := zip.NewWriter(r.file)
	defer arc.Close()

	t := time.Now()

	names, manifest := prepareManifest(r.paths)
	if err := saveFile(arc, "MANIFEST", t, manifest); err != nil {
		return err
	}

	// in the same order as in manifest
	for _, name := range names {
		path := r.paths[name]
		// ignoring absent files
		if info, err := os.Stat(path); err == nil {
			switch {
			case info.Mode().IsRegular():
				var r io.ReadCloser
				if r, err = os.Open(path); err != nil {
					return err
				}
				if err := saveFile(arc, name, info.ModTime(), r); err != nil {
					r.Close()
					return err
				}
				r.Close()
			case info.Mode().IsDir():
				if err := saveDir(arc, name, path); err != nil {
					return err
				}
			default:
			}
		}
	}
	return nil
}

func prepareManifest(paths map[string]string) ([]string, *bytes.Buffer) {

	buf := new(bytes.Buffer)
	if len(paths) == 0 {
		return nil, buf
	}

	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		buf.WriteString(fmt.Sprintf("%s\t%s\n", k, paths[k]))
	}
	return keys, buf
}

func saveFile(dst *zip.Writer, name string, t time.Time, src io.Reader) error {

	var (
		w   io.Writer
		err error
	)
	if w, err = dst.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: t}); err != nil {
		return err
	}
	if _, err = io.Copy(w, src); err != nil {
		return err
	}
	return nil
}

func saveDir(dst *zip.Writer, name, dir string) error {
	saveFile := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		// Get the path of the file relative to the source folder
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		// root entry under new name
		rel = filepath.ToSlash(filepath.Join(name, rel))

		var r io.ReadCloser
		if r, err = os.Open(path); err != nil {
			return err
		}
		defer r.Close()

		if err = saveFile(dst, rel, info.ModTime(), r); err != nil {
			return err
		}
		return nil
	}
	if err := filepath.Walk(dir, saveFile); err != nil {
		return err
	}
	return nil
}
