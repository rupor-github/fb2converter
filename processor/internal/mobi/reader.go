package mobi

import (
	"bytes"
	"image"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"go.uber.org/zap"
)

// Reader - mobi thumbnail extractor.
type Reader struct {
	log     *zap.Logger
	width   int
	height  int
	stretch bool
	fname   string
	//
	acr       []byte
	asin      []byte
	cdetype   []byte
	cdekey    []byte
	thumbnail []byte
}

// NewReader returns pointer to Reader with parsed mobi file.
func NewReader(fname string, w, h int, stretch bool, log *zap.Logger) (*Reader, error) {

	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	r := &Reader{log: log, fname: fname, width: w, height: h, stretch: stretch}
	r.produceThumbnail(data)

	return r, nil
}

// SaveResult saves mobi thumbnail to the requested location.
func (r *Reader) SaveResult(dir string) (bool, error) {

	if len(r.thumbnail) == 0 {
		r.log.Debug("Nothing to save - no cover or thumbnail extracted", zap.String("file", r.fname))
		return false, nil
	}

	asin := string(r.asin)
	if len(r.cdekey) > 0 {
		asin = string(r.cdekey)
	}
	if len(asin) == 0 {
		r.log.Debug("Nothing to save - document has no ASIN", zap.String("file", r.fname))
		return false, nil
	}

	fname := filepath.Join(dir, "thumbnail_"+asin+"_"+string(r.cdetype)+"_portrait.jpg")
	if _, err := os.Stat(fname); err == nil {
		r.log.Debug("Overwriting existing thumbnail", zap.String("file", r.fname), zap.String("thumb", fname))
	}

	if err := ioutil.WriteFile(fname, r.thumbnail, 0644); err != nil {
		return false, err
	}

	r.log.Debug("Thumbnail created", zap.String("ASIN", asin), zap.String("file", r.fname), zap.String("thumb", fname))
	return true, nil
}

func (r *Reader) produceThumbnail(data []byte) {

	rec0 := readSection(data, 0)

	if getInt16(rec0, cryptoType) != 0 {
		r.log.Debug("Encrypted book", zap.String("file", r.fname))
		return
	}

	var (
		kf8    int
		kfrec0 []byte
	)

	kf8off := readExth(rec0, exthKF8Offset)
	if len(kf8off) > 0 {
		// only pay attention to first KF8 offfset - there should only be one
		if kf8 = getInt32(kf8off[0], 0); kf8 >= 0 {
			kfrec0 = readSection(data, kf8)
		}
	}
	combo := len(kfrec0) > 0 && kf8 >= 0

	// save ACR
	const alphabet = `- ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789`
	r.acr = bytes.Map(func(sym rune) rune {
		if sym == 0 {
			return -1
		}
		if strings.ContainsRune(alphabet, sym) {
			return sym
		}
		return '_'
	}, data[0:32])

	exth := readExth(rec0, exthASIN)
	if len(exth) > 0 {
		r.asin = exth[0]
	}
	exth = readExth(rec0, exthCDEType)
	if len(exth) > 0 {
		r.cdetype = exth[0]
	}
	exth = readExth(rec0, exthCDEContentKey)
	if len(exth) > 0 {
		r.cdekey = exth[0]
	}

	firstimage := getInt32(rec0, firstRescRecord)
	exthCover := readExth(rec0, exthCoverOffset)
	coverIndex := -1
	if len(exthCover) > 0 {
		coverIndex = getInt32(exthCover[0], 0)
		coverIndex += firstimage
	}

	exthThumb := readExth(rec0, exthThumbOffset)
	thumbIndex := -1
	if len(exthThumb) > 0 {
		thumbIndex = getInt32(exthThumb[0], 0)
		thumbIndex += firstimage
	}

	if coverIndex >= 0 {
		var (
			img image.Image
			err error
		)
		w, h := 0, 0
		if thumbIndex >= 0 {
			thumb := readSection(data, thumbIndex)
			if img, _, err = image.Decode(bytes.NewReader(thumb)); err == nil {
				w, h = img.Bounds().Dx(), img.Bounds().Dy()
			} else {
				r.log.Debug("Unable to encode extracted thumbnail", zap.String("file", r.fname), zap.Error(err))
				img = nil
			}
		}
		if img != nil && (w > r.width || h > r.height) && !r.stretch {
			// always convert to JPEG
			var buf = new(bytes.Buffer)
			if err = imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(75)); err == nil {
				buf, _ = SetJpegDPI(buf, DpiPxPerInch, 300, 300)
				r.thumbnail = buf.Bytes()
			} else {
				r.log.Debug("Unable to encode extracted thumbnail", zap.String("file", r.fname), zap.Error(err))
			}
		} else {
			// recreate thumnail from cover image if possible
			thumb := readSection(data, coverIndex)
			if img, _, err = image.Decode(bytes.NewReader(thumb)); err == nil {
				if imgthumb := imaging.Thumbnail(img, r.width, r.height, imaging.Lanczos); imgthumb != nil {
					var buf = new(bytes.Buffer)
					if err = imaging.Encode(buf, imgthumb, imaging.JPEG, imaging.JPEGQuality(75)); err == nil {
						buf, _ = SetJpegDPI(buf, DpiPxPerInch, 300, 300)
						r.thumbnail = buf.Bytes()
					} else {
						r.log.Debug("Unable to encode produced thumbnail", zap.String("file", r.fname), zap.Error(err))
					}
				} else {
					r.log.Debug("Unable to resize extracted cover", zap.String("file", r.fname))
				}
			} else {
				r.log.Debug("Unable to decode extracted cover", zap.String("file", r.fname), zap.Error(err))
			}
		}
	}

	if combo {
		// always prefer data from KF8
		exth = readExth(kfrec0, exthASIN)
		if len(exth) > 0 {
			r.asin = exth[0]
		}
		exth = readExth(kfrec0, exthCDEType)
		if len(exth) > 0 {
			r.cdetype = exth[0]
		}
		exth = readExth(kfrec0, exthCDEContentKey)
		if len(exth) > 0 {
			r.cdekey = exth[0]
		}
	}
}
