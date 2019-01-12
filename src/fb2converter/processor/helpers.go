package processor

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"fb2converter/static"
)

func (p *Processor) getStylesheet() (*dataFile, error) {

	var (
		err error
		d   = &dataFile{
			id: "style",
			ct: "text/css",
		}
	)

	fname := p.env.Cfg.Doc.Stylesheet
	if len(fname) > 0 && len(p.env.Cfg.Path) > 0 {
		if !filepath.IsAbs(fname) {
			fname = filepath.Join(p.env.Cfg.Path, fname)
		}
		if d.data, err = ioutil.ReadFile(fname); err != nil {
			return nil, errors.Wrap(err, "unable to read stylesheet")
		}
	} else {
		if dir, err := static.AssetDir(DirProfile); err == nil {
			name := fmt.Sprintf("default.%s.css", p.format)
			for _, a := range dir {
				if a == name {
					fname = name
					break
				}
			}
		}
		if len(fname) == 0 {
			fname = "default.css"
		}
		if d.data, err = static.Asset(path.Join(DirProfile, fname)); err != nil {
			return nil, errors.Wrap(err, "unable to get default stylesheet")
		}
	}
	d.fname = "stylesheet.css"
	d.relpath = DirContent
	p.Book.Data = append(p.Book.Data, d)
	return d, nil
}

// getDefaultCover returns binary element for "default" cover image if one is configured or built in otherwise.
func (p *Processor) getDefaultCover(i int) (*binary, error) {

	var (
		err error
		b   = &binary{
			log:     p.env.Log,
			relpath: filepath.Join(DirContent, DirImages),
		}
	)

	fname := p.env.Cfg.Doc.Cover.ImagePath
	if len(fname) > 0 && len(p.env.Cfg.Path) > 0 {
		if !filepath.IsAbs(fname) {
			fname = filepath.Join(p.env.Cfg.Path, fname)
		}
		// NOTE: I do not want to make sure that supplied default cover has right properties, will just use it as is
		if b.data, err = ioutil.ReadFile(fname); err != nil {
			return nil, errors.Wrap(err, "unable to read cover image")
		}
		ext := filepath.Ext(fname)
		b.fname = fmt.Sprintf("bin%08d%s", i, ext)
		b.ct = mime.TypeByExtension(ext)
		b.imgType = strings.TrimPrefix(ext, ".")
	} else {
		fname := "default_cover.jpeg"
		if b.data, err = static.Asset(fname); err != nil {
			return nil, errors.Wrap(err, "unable to get default cover image")
		}
		b.fname = fmt.Sprintf("bin%08d.jpeg", i)
		b.ct = mime.TypeByExtension(".jpeg")
		b.imgType = "jpeg"
	}

	b.img, b.imgType, err = image.Decode(bytes.NewReader(b.data))
	if err != nil {
		return nil, errors.Wrapf(err, "bad default cover image %s", fname)
	}
	b.id = "dummycover"
	return b, nil
}

// getNotFoundImage returns binary element for "not found" image.
func (p *Processor) getNotFoundImage(i int) (*binary, error) {

	var (
		err error
		b   = &binary{
			log:     p.env.Log,
			relpath: filepath.Join(DirContent, DirImages),
		}
	)

	if b.data, err = static.Asset(path.Join(DirResources, "not_found.png")); err != nil {
		return nil, errors.Wrap(err, "unable to get image not_found.png")
	}
	b.fname = fmt.Sprintf("bin%08d.png", i)
	b.ct = mime.TypeByExtension(".png")

	b.img, b.imgType, err = image.Decode(bytes.NewReader(b.data))
	if err != nil {
		return nil, errors.Wrap(err, "bad image not_found.png")
	}
	b.id = "notfound"
	return b, nil
}

// getVignetteFile returns name of the file for a particular vignette and if possible caches vignette image.
func (p *Processor) getVignetteFile(level, vignette string) string {

	const empty = ""

	if !p.env.Cfg.Doc.Vignettes.Create || len(p.env.Cfg.Doc.Vignettes.Images) == 0 {
		return empty
	}

	var (
		req, def bool
		fname    string
		l        map[string]string
	)
	l, req = p.env.Cfg.Doc.Vignettes.Images[level]
	if !req {
		l, def = p.env.Cfg.Doc.Vignettes.Images["default"]
		if !def {
			return empty
		}
	}
	fname, req = l[vignette]
	if !req {
		if def {
			return empty
		}
		if l, def = p.env.Cfg.Doc.Vignettes.Images["default"]; !def {
			return empty
		}
		if fname, req = l[vignette]; !req {
			return empty
		}
	}
	if strings.EqualFold(fname, "none") {
		return empty
	}

	// see if we already have vignette file
	for _, b := range p.Book.Vignettes {
		if b.id == fname {
			return fname
		}
	}

	b := &binary{log: p.env.Log}

	var err error
	if len(p.env.Cfg.Path) > 0 {
		// try disk first
		absname := fname
		if !filepath.IsAbs(absname) {
			absname = filepath.Join(p.env.Cfg.Path, absname)
		}
		if b.data, err = ioutil.ReadFile(absname); err != nil {
			b.data = nil
		}
	}
	if len(b.data) == 0 {
		// see if we have suitable built-in
		if b.data, err = static.Asset(fname); err != nil {
			b.data = nil
		}
	}

	if len(b.data) == 0 {
		p.env.Log.Warn("unable to get vignette",
			zap.String("level", level),
			zap.String("vignette", vignette),
			zap.String("file", fname))
		return empty
	}

	b.id = fname
	b.fname = filepath.Base(fname)
	b.relpath = filepath.Join(DirContent, DirVignettes)
	b.ct = mime.TypeByExtension(filepath.Ext(fname))
	p.Book.Vignettes = append(p.Book.Vignettes, b)

	return fname
}

// isTTFFontFile returns true is file is True Type font - based on the file name.
func isTTFFontFile(path string, buf []byte) bool {
	return strings.EqualFold(filepath.Ext(path), ".ttf") && filetype.Is(buf, "ttf")
}

// isOTFFontFile returns true is file is opentype font - based on the file name.
func isOTFFontFile(path string, buf []byte) bool {
	return strings.EqualFold(filepath.Ext(path), ".otf") && filetype.Is(buf, "otf")
}

var fmts = [...]string{"gif", "bmp", "jpeg", "png"}

// isImageSupported returns true if image is supported and does not need conversion.
func isImageSupported(format string) bool {

	imgType := strings.TrimPrefix(format, ".")
	for _, t := range fmts {
		if strings.EqualFold(t, imgType) {
			return true
		}
	}
	return false
}

// CopyFile simply copes file from src to dst. No checking is done.
func CopyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
