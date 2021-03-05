// Package static has runtime resources for various commands.
package static

import (
	"embed"
	"os"
	"path"
	"path/filepath"
)

//go:embed configuration.toml default_cover.jpeg dictionaries profiles resources sentences
var content embed.FS

// -------------------------------------------------------------------------------------------------------------------------
// Following functions provide minimal emulation of go-bindata generated code to avoid changing the rest of the program.
// NOTE: this is not a generic implementation - it reliys on details of the usage!
// -------------------------------------------------------------------------------------------------------------------------

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or could not be loaded.
// It emulates fuction form go_bindata generated code to avoid changes related to move to go embed.
func Asset(name string) ([]byte, error) {
	return content.ReadFile(name)
}

// AssetDir returns the file names below a certain directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
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
		entries = append(entries, de.Name())
	}
	return entries, nil
}

func restoreFile(dir, name string) error {

	data, err := content.ReadFile(name)
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
		err := RestoreAssets(dir, path.Join(name, de.Name()))
		if err != nil {
			return err
		}
	}
	return nil
}
