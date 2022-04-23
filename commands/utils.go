package commands

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
	"golang.org/x/text/transform"
)

// isArchiveFile detects if file is our supported archive.
func isArchiveFile(fname string) (bool, error) {

	if !strings.EqualFold(filepath.Ext(fname), ".zip") {
		return false, nil
	}

	file, err := os.Open(fname)
	if err != nil {
		return false, err
	}
	defer file.Close()

	header := make([]byte, 262)
	if count, err := file.Read(header); err != nil {
		return false, err
	} else if count < 262 {
		return false, nil
	}
	return filetype.Is(header, "zip"), nil
}

// isEpubFile detects if file is our supported archive.
func isEpubFile(fname string) (bool, error) {

	if !strings.EqualFold(filepath.Ext(fname), ".epub") {
		return false, nil
	}

	file, err := os.Open(fname)
	if err != nil {
		return false, err
	}
	defer file.Close()

	header := make([]byte, 262)
	if count, err := file.Read(header); err != nil {
		return false, err
	} else if count < 262 {
		return false, nil
	}
	return filetype.Is(header, "epub"), nil
}

type srcEncoding int

const (
	encUnknown srcEncoding = iota
	encUTF8
	encUTF16BigEndian
	encUTF16LittleEndian
	encUTF32BigEndian
	encUTF32LittleEndian
)

// selectReader handles various unicode encodings (with or without BOM).
func selectReader(r io.Reader, enc srcEncoding) io.Reader {
	switch enc {
	case encUnknown:
		return r
	case encUTF8:
		return transform.NewReader(r, unicode.BOMOverride(unicode.UTF8.NewDecoder()))
	case encUTF16BigEndian:
		return transform.NewReader(r, unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder())
	case encUTF16LittleEndian:
		return transform.NewReader(r, unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder())
	case encUTF32BigEndian:
		return transform.NewReader(r, utf32.UTF32(utf32.BigEndian, utf32.ExpectBOM).NewDecoder())
	case encUTF32LittleEndian:
		return transform.NewReader(r, utf32.UTF32(utf32.LittleEndian, utf32.ExpectBOM).NewDecoder())
	default:
		panic("unsupported encoding - should never happen")
	}
}

func isUTF32BigEndianBOM4(buf []byte) bool {
	return buf[0] == 0x00 && buf[1] == 0x00 && buf[2] == 0xFE && buf[3] == 0xFF
}

func isUTF32LittleEndianBOM4(buf []byte) bool {
	return buf[0] == 0xFF && buf[1] == 0xFE && buf[2] == 0x00 && buf[3] == 0x00
}

func isUTF8BOM3(buf []byte) bool {
	return buf[0] == 0xEF && buf[1] == 0xBB && buf[2] == 0xBF
}

func isUTF16BigEndianBOM2(buf []byte) bool {
	return buf[0] == 0xFE && buf[1] == 0xFF
}

func isUTF16LittleEndianBOM2(buf []byte) bool {
	return buf[0] == 0xFF && buf[1] == 0xFE
}

func detectUTF(buf []byte) (enc srcEncoding) {

	if isUTF32BigEndianBOM4(buf) {
		return encUTF32BigEndian
	}
	if isUTF32LittleEndianBOM4(buf) {
		return encUTF32LittleEndian
	}
	if isUTF8BOM3(buf) {
		return encUTF8
	}
	if isUTF16BigEndianBOM2(buf) {
		return encUTF16BigEndian
	}
	if isUTF16LittleEndianBOM2(buf) {
		return encUTF16LittleEndian
	}
	return encUnknown
}

// isBookFile detects if file is fb2/xml file and if it is tries to detect its encoding.
func isBookFile(fname string) (bool, srcEncoding, error) {

	if !strings.EqualFold(filepath.Ext(fname), ".fb2") {
		return false, encUnknown, nil
	}

	file, err := os.Open(fname)
	if err != nil {
		return false, encUnknown, err
	}
	defer file.Close()

	buf := []byte{1, 1, 1, 1}
	_, err = file.Read(buf)
	if err != nil {
		return false, encUnknown, err
	}
	enc := detectUTF(buf)
	if ref, err := file.Seek(0, 0); err != nil {
		return false, encUnknown, err
	} else if ref != 0 {
		return false, encUnknown, fmt.Errorf("unable reset file: %s", fname)
	}

	header := make([]byte, 512)
	if _, err := selectReader(file, enc).Read(header); err != nil {
		return false, encUnknown, err
	}
	return filetype.Is(header, "fb2"), enc, nil
}

// isBookInArchive detects if compressed file is fb2/xml file and if it is tries to detect its encoding.
func isBookInArchive(f *zip.File) (bool, srcEncoding, error) {

	if !strings.EqualFold(filepath.Ext(f.FileHeader.Name), ".fb2") {
		return false, encUnknown, nil
	}

	r, err := f.Open()
	if err != nil {
		return false, encUnknown, err
	}

	buf := []byte{1, 1, 1, 1}
	_, err = r.Read(buf)
	if err != nil {
		r.Close()
		return false, encUnknown, err
	}
	enc := detectUTF(buf)
	r.Close()

	r, err = f.Open()
	if err != nil {
		return false, encUnknown, err
	}
	defer r.Close()

	header := make([]byte, 512)
	if _, err := selectReader(r, enc).Read(header); err != nil {
		return false, encUnknown, err
	}
	return filetype.Is(header, "fb2"), enc, nil
}

func init() {
	// Register FB2 matcher for filetype
	filetype.AddMatcher(
		filetype.NewType("fb2", "application/x-fictionbook+xml"),
		func(buf []byte) bool {
			text := string(buf)
			return strings.HasPrefix(text, `<?xml`) && strings.Contains(text, `<FictionBook`)
		})
}
