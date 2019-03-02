// Copyright 2018 hidez8891. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	bytesEX "github.com/hidez8891/zip/internal/bytes"
)

// A WriteCloser implements the io.WriteCloser
type WriteCloser struct {
	writer io.Writer
	closer io.Closer
}

// Write implements the io.WriteCloser interface.
func (w *WriteCloser) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

// Close implements the io.WriteCloser interface.
func (w *WriteCloser) Close() error {
	return w.closer.Close()
}

// Updater provides editing of zip files.
type Updater struct {
	files   []string
	headers map[string]*FileHeader
	entries map[string]*bytesEX.BufferAt
	r       *Reader
	Comment string
}

// NewUpdater returns a new Updater from r and size.
func NewUpdater(r io.ReaderAt, size int64) (*Updater, error) {
	zr, err := NewReader(r, size)
	if err != nil {
		return nil, err
	}

	files := make([]string, len(zr.File))
	headers := make(map[string]*FileHeader, len(zr.File))
	for i, zf := range zr.File {
		files[i] = zf.Name
		headers[zf.Name] = &zf.FileHeader
	}

	return &Updater{
		files:   files,
		headers: headers,
		entries: make(map[string]*bytesEX.BufferAt),
		r:       zr,
		Comment: zr.Comment,
	}, nil
}

// Files returns a FileHeader list.
func (u *Updater) Files() []*FileHeader {
	files := make([]*FileHeader, len(u.files))
	for i, name := range u.files {
		files[i] = u.headers[name]
	}
	return files
}

// Open returns a ReadCloser that provides access to the File's contents.
func (u *Updater) Open(name string) (io.ReadCloser, error) {
	if _, ok := u.headers[name]; !ok {
		return nil, errors.New("File not found")
	}

	if buf, ok := u.entries[name]; ok {
		b := buf.Bytes()
		z, err := NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return nil, err
		}
		return z.File[0].Open()
	}

	for _, zf := range u.r.File {
		if zf.Name == name {
			return zf.Open()
		}
	}
	return nil, errors.New("internal error: name not found")
}

// Create returns a Writer to which the file contents should be written.
func (u *Updater) Create(name string) (io.WriteCloser, error) {
	if _, ok := u.headers[name]; ok {
		return nil, errors.New("invalid duplicate file name")
	}

	u.entries[name] = new(bytesEX.BufferAt)
	z := NewWriter(u.entries[name])

	w, err := z.Create(name)
	if err != nil {
		return nil, err
	}
	u.files = append(u.files, name)
	u.headers[name] = z.dir[0].FileHeader

	wc := &WriteCloser{
		writer: w,
		closer: z,
	}
	return wc, nil
}

// Update returns a Writer to which the file contents should be overwritten.
func (u *Updater) Update(name string) (io.WriteCloser, error) {
	if _, ok := u.headers[name]; !ok {
		return nil, errors.New("not found file name")
	}
	useDataDescriptor := u.headers[name].Flags&FlagDataDescriptor != 0

	u.entries[name] = new(bytesEX.BufferAt)
	z := NewWriter(u.entries[name])

	w, err := z.CreateHeader(u.headers[name])
	if err != nil {
		return nil, err
	}
	if !useDataDescriptor {
		z.dir[0].FileHeader.Flags &^= FlagDataDescriptor
	}
	u.headers[name] = z.dir[0].FileHeader

	wc := &WriteCloser{
		writer: w,
		closer: z,
	}
	return wc, nil
}

// Rename changes the file name.
func (u *Updater) Rename(oldName, newName string) error {
	if _, ok := u.headers[newName]; ok {
		return errors.New("new file name already exists")
	}

	header, ok := u.headers[oldName]
	if !ok {
		return errors.New("not found file name")
	}
	header.Name = newName
	u.headers[newName] = header
	delete(u.headers, oldName)

	for i, v := range u.files {
		if v == oldName {
			u.files[i] = newName
		}
	}

	if entry, ok := u.entries[oldName]; ok {
		u.entries[newName] = entry
		delete(u.entries, oldName)
	}
	return nil
}

// Remove deletes the file.
func (u *Updater) Remove(name string) error {
	if _, ok := u.headers[name]; !ok {
		return errors.New("not found file name")
	}
	delete(u.headers, name)

	newfiles := make([]string, 0)
	for _, v := range u.files {
		if v != name {
			newfiles = append(newfiles, v)
		}
	}
	u.files = newfiles

	if _, ok := u.entries[name]; ok {
		delete(u.entries, name)
	}
	return nil
}

// SaveAs saves the changes to w.
// If data descriptor is not used, w must implement io.WriterAt.
func (u *Updater) SaveAs(w io.Writer) error {
	z := NewWriter(w)

	if err := z.SetComment(u.Comment); err != nil {
		return err
	}

	for _, name := range u.files {
		offset := z.cw.count

		fh := u.headers[name]
		if err := writeHeader(z.cw, fh); err != nil {
			return err
		}
		z.dir = append(z.dir, &header{
			FileHeader: fh,
			offset:     uint64(offset),
		})

		var zfile *File
		if entry, ok := u.entries[name]; ok {
			// write new file
			zr, err := NewReader(bytes.NewReader(entry.Bytes()), int64(entry.Len()))
			if err != nil {
				return err
			}
			if len(zr.File) == 0 {
				return fmt.Errorf("internal error: %s is not exist", name)
			}
			zfile = zr.File[0]
		} else {
			// write zip's content
			for _, zf := range u.r.File {
				if zf.Name == name {
					zfile = zf
				}
			}
			if zfile == nil {
				return fmt.Errorf("internal error: %s is not exist", name)
			}
		}

		size := int64(zfile.CompressedSize64)
		if zfile.Flags&FlagDataDescriptor != 0 {
			if fh.isZip64() {
				size += dataDescriptor64Len
			} else {
				size += dataDescriptorLen
			}
		}
		bodyOffset, err := zfile.findBodyOffset()
		if err != nil {
			return err
		}
		r := io.NewSectionReader(zfile.zipr, zfile.headerOffset+bodyOffset, size)
		if _, err := io.Copy(z.cw, r); err != nil {
			return err
		}
	}

	return z.Close()
}

// Sort updates the file name list to the output of f.
func (u *Updater) Sort(f func([]string) []string) error {
	files := f(u.files)

	if len(files) != len(u.files) {
		return errors.New("files length are different")
	}

	exists := make(map[string]bool)
	for _, name := range files {
		exists[name] = true
	}
	for _, name := range u.files {
		if _, ok := exists[name]; !ok {
			return fmt.Errorf("%s is not found in new files", name)
		}
	}

	u.files = files
	return nil
}

// Cancel discards the changes and ends editing.
func (u *Updater) Cancel() error {
	u.files = make([]string, 0)
	u.headers = make(map[string]*FileHeader, 0)
	u.entries = make(map[string]*bytesEX.BufferAt, 0)
	u.r = nil
	return nil
}

// Close discards the changes and ends editing.
func (u *Updater) Close() error {
	return u.Cancel()
}
