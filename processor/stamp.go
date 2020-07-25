package processor

import (
	"fmt"
	"image"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/rupor-github/fb2converter/static"
)

func (p *Processor) stampCover(im image.Image) (image.Image, error) {

	if p.stampPlacement == StampNone {
		// just in case
		return nil, nil
	}

	p.env.Log.Debug("Stamping cover - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Stamping cover - done",
			zap.Duration("elapsed", time.Since(start)),
		)
	}(time.Now())

	titles := make([]string, 0, 3)

	titles = append(titles, p.Book.Title)
	var series string
	if len(p.Book.SeqName) > 0 {
		series = p.Book.SeqName
		if p.Book.SeqNum != 0 {
			series = fmt.Sprintf("%s: %d", series, p.Book.SeqNum)
		}
		if len(series) > 0 {
			titles = append(titles, series)
		}
	}
	author := p.Book.BookAuthors(p.env.Cfg.Doc.AuthorFormat, true)
	if len(author) > 0 {
		titles = append(titles, author)
	}

	// tuning
	fh := float64(im.Bounds().Dy()) / 4 / 6
	if fh < 10 {
		fh = 10
	}
	off := fh / 4

	dc := gg.NewContextForImage(im)

	// prepare font
	if len(p.env.Cfg.Doc.Cover.Font) > 0 {
		absname := p.env.Cfg.Doc.Cover.Font
		if !filepath.IsAbs(absname) {
			absname = filepath.Join(p.env.Cfg.Path, absname)
		}
		if err := dc.LoadFontFace(absname, fh); err != nil {
			// misconfiguration - get out
			return nil, err
		}
	} else {
		data, err := static.Asset(path.Join(DirResources, "LinLibertine_RBah.ttf"))
		if err != nil {
			return nil, errors.Wrap(err, "unable to get default stamp font")
		}
		f, err := truetype.Parse(data)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse default stamp font")
		}
		face := truetype.NewFace(f, &truetype.Options{
			Size: fh,
			// Hinting: font.HintingFull,
		})
		dc.SetFontFace(face)
	}

	var x, y, w, h = float64(0), float64(0), float64(im.Bounds().Dx()), float64(im.Bounds().Dy()) / 4
	switch p.stampPlacement {
	case StampTop:
		x, y = 0, 0
	case StampMiddle:
		x, y = 0, (float64(im.Bounds().Dy())-h)/2
	case StampBottom:
		x, y = 0, float64(im.Bounds().Dy())-h
	default:
		panic("unexpected stamp placement - should never happen")
	}

	text := strings.Join(titles, "\n")
	if len(text) > 0 {
		dc.DrawRectangle(x, y, w, h)
		dc.SetRGBA(0, 0, 0, 0.2)
		dc.Fill()
		dc.SetRGB(255, 255, 255)
		dc.DrawStringWrapped(text, w/2, y, 0.5, 0, w-off, 1, gg.AlignCenter)
		return dc.Image(), nil
	}
	return nil, nil
}
