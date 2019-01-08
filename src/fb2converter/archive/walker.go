package archive

import (
	"archive/zip"
	"strings"
)

// WalkFunc is the type of the function called for each file in archive
// visited by Walk. The archive argument contains path to archive passed to Walk
// The file argument is the zip.File structure for file in archive which satisfies
// match condition. If an error is returned, processing stops.
type WalkFunc func(archive string, file *zip.File) error

// Walk walks the all files in the archive which satisfy match condition,
// calling walkFn for each item.
func Walk(archive, pattern string, walkFn WalkFunc) error {

	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if !f.FileInfo().IsDir() && strings.HasPrefix(f.FileHeader.Name, pattern) {
			if err := walkFn(archive, f); err != nil {
				return err
			}
		}
	}
	return nil
}
