// Package static has runtime resources for various commands.
package static

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed configuration.toml default_cover.jpeg dictionaries profiles resources sentences
var content embed.FS

// -------------------------------------------------------------------------------------------------------------------------
// Following functions provide minimal emulation of go-bindata generated code to avoid changing the rest of the program.
// Individual files could be gzip compressed - this slows down access but otherwise transparent to the outside code.
// NOTE: this is not a generic implementation - it relies on details of the usage!
// -------------------------------------------------------------------------------------------------------------------------

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or could not be loaded.
func Asset(name string) ([]byte, error) {

	data, err := content.ReadFile(name)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		data, err = content.ReadFile(name + ".gz")
		if err != nil {
			return nil, err
		}
		gzr, err := gzip.NewReader(bytes.NewBuffer(data))
		if err != nil {
			return nil, err
		}
		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, gzr); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	return data, nil
}

// AssetDir returns the file names below a certain directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//
//	data/
//	  foo.txt
//	  img/
//	    a.png
//	    b.png
//
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {

	name = path.Clean(filepath.ToSlash(name))

	dirEntries, err := content.ReadDir(name)
	if err != nil {
		return nil, err
	}

	var entries []string
	for _, de := range dirEntries {
		entries = append(entries, strings.TrimSuffix(de.Name(), ".gz"))
	}
	return entries, nil
}

func restoreFile(dir, name string) error {

	data, err := Asset(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), os.FileMode(0755)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, os.FileMode(0644)); err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively.
func RestoreAssets(dir, name string) error {

	dir, name = path.Clean(filepath.ToSlash(dir)), path.Clean(filepath.ToSlash(name))

	dirEntries, err := content.ReadDir(name)
	if err != nil {
		return restoreFile(dir, name)
	}

	for _, de := range dirEntries {
		err := RestoreAssets(dir, path.Join(name, strings.TrimSuffix(de.Name(), ".gz")))
		if err != nil {
			return err
		}
	}
	return nil
}
