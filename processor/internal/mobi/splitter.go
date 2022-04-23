package mobi

// Despite obvious ineffectiveness I decided to repeat python code "ad verbum" for now, it is very time
// consuming to debug Amazon issues and incompatibilities and old code seems to be working well.
// Visit KindleUnpack - https://github.com/kevinhendricks/KindleUnpack for any information.

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Splitter - mobi splitter annd optimizer.
type Splitter struct {
	log   *zap.Logger
	combo bool
	//
	contentGUID string
	acr         []byte
	asin        []byte
	cdetype     []byte
	cdekey      []byte
	pagedata    []byte
	result      []byte
}

// NewSplitter returns pointer to Slitter with parsed mobi file.
func NewSplitter(fname string, u uuid.UUID, asin string, combo, nonPersonal, forceASIN bool, log *zap.Logger) (*Splitter, error) {

	data, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	s := &Splitter{
		log:         log,
		combo:       combo,
		contentGUID: strings.Replace(u.String(), "-", "", -1)[:8],
	}

	var id []byte
	if len(asin) == 0 {
		id = convertToRadix32(strings.Replace(u.String(), "-", "", -1), 10)
	} else {
		id = []byte(asin)
	}

	if combo {
		s.produceCombo(data, id, nonPersonal)
	} else {
		s.produceKF8(data, id, nonPersonal, forceASIN)
	}
	return s, nil
}

// SaveResult saves combo mobi to the requested location.
func (s *Splitter) SaveResult(fname string) error {
	if len(s.result) == 0 {
		return errors.New("nothing to save")
	}
	return os.WriteFile(fname, s.result, 0644)
}

// SavePageMap saves combo mobi to the requested location.
func (s *Splitter) SavePageMap(fname string, eink bool) error {

	if len(s.result) == 0 {
		s.log.Debug("Page map does not exist, ignoring")
		return nil
	}

	dir := filepath.Dir(fname)
	base := strings.TrimSuffix(filepath.Base(fname), filepath.Ext(fname))

	if eink {
		dir = filepath.Join(dir, base+".sdr")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("unable to create pagemap directory: %w", err)
		}
	}
	base += ".apnx"
	return os.WriteFile(filepath.Join(dir, base), s.pagedata, 0644)
}

func (s *Splitter) produceCombo(data []byte, u []byte, nonPersonal bool) {

	rec0 := readSection(data, 0)
	if a := getInt32(rec0, mobiVersion); a == 8 {
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
	if kf8 < 0 || len(kfrec0) == 0 {
		return
	}

	// save ACR
	const alphabet = `- ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789`
	s.acr = bytes.Map(func(sym rune) rune {
		if sym == 0 {
			return -1
		}
		if strings.ContainsRune(alphabet, sym) {
			return sym
		}
		return '_'
	}, data[0:32])

	// store data for page map
	var pdata []byte
	srcs, numSrcs := getInt32(rec0, firstNonText), getInt16(data, numberOfPdbRecords)
	if srcs >= 0 && numSrcs > 0 {
		for i := srcs; i < numSrcs; i++ {
			d := readSection(data, i)
			if bytes.Equal(d[0:4], []byte("PAGE")) {
				pdata = d
			}
		}
	}

	result := make([]byte, len(data))
	copy(result, data)

	// check if there are SRCS records and eliminate them
	srcs, numSrcs = getInt32(rec0, srcsIndex), getInt32(rec0, srcsCount)
	if srcs >= 0 && numSrcs > 0 {
		for i := srcs; i < srcs+numSrcs; i++ {
			result = nullSection(result, i)
		}
		putInt32(rec0, srcsIndex, -1)
		putInt32(rec0, srcsCount, 0)
	}

	asin := readExth(rec0, exthASIN)
	if len(asin) == 0 {
		s.asin = u
	} else {
		s.asin = asin[0]
	}
	if nonPersonal {
		rec0 = addExth(rec0, exthCDEType, []byte("EBOK"))
		if len(asin) == 0 {
			rec0 = addExth(rec0, exthASIN, s.asin)
		}
	}
	result = writeSection(result, 0, rec0)

	// Only keep the correct Start Reading offset, KG 2.5 carries over the one from the mobi7 part, which then
	// points at garbage in the mobi8 part, and confuses FW 3.4
	kf8starts := readExth(kfrec0, exthStartReading)
	kf8count := len(kf8starts)
	for kf8count > 1 {
		kf8count--
		kfrec0 = delExth(kfrec0, exthStartReading)
	}

	firstimage := getInt32(rec0, firstRescRecord)
	exthCover := readExth(kfrec0, exthCoverOffset)
	coverIndex := -1
	if len(exthCover) > 0 {
		coverIndex = getInt32(exthCover[0], 0)
		coverIndex += firstimage
	}

	exthThumb := readExth(kfrec0, exthThumbOffset)
	thumbIndex := -1
	if len(exthThumb) > 0 {
		thumbIndex = getInt32(exthThumb[0], 0)
		thumbIndex += firstimage
	}

	if coverIndex >= 0 {
		if thumbIndex >= 0 {
			// make sure embedded thumbnail has the right size
			coverImage := readSection(data, coverIndex)
			if img, _, err := image.Decode(bytes.NewReader(coverImage)); err == nil {
				if thumb := imaging.Thumbnail(img, 330, 470, imaging.Lanczos); thumb != nil {
					var buf = new(bytes.Buffer)
					if err := imaging.Encode(buf, thumb, imaging.JPEG, imaging.JPEGQuality(75)); err != nil {
						s.log.Error("Unable to encode processed thumbnail, skipping", zap.Error(err))
					} else {
						var jfifAdded bool
						buf, jfifAdded = SetJpegDPI(buf, DpiPxPerInch, 300, 300)
						if jfifAdded {
							s.log.Debug("Inserting JFIF APP0 marker segment into mobi thumbnail")
						}
						result = writeSection(result, thumbIndex, buf.Bytes())
					}
				}
			}
		} else {
			// old trick, set thumbnail to the cover image, use as a last resort
			kfrec0 = addExth(kfrec0, exthThumbOffset, exthCover[0])
			thumbIndex = coverIndex
		}
		exthrec := readExth(kfrec0, exthThumbnailURI)
		if len(exthrec) > 0 {
			kfrec0 = delExth(kfrec0, exthThumbnailURI)
		}

		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(thumbIndex-firstimage))

		var uri bytes.Buffer
		uri.WriteString("kindle:embed:")
		uri.Write(convertToRadix32(hex.EncodeToString(buf), 4))

		kfrec0 = addExth(kfrec0, exthThumbnailURI, uri.Bytes())
	}

	cdekey := readExth(kfrec0, exthCDEContentKey)
	if len(cdekey) == 0 {
		s.cdekey = u
	} else {
		s.cdekey = cdekey[0]
	}
	if nonPersonal {
		s.cdetype = []byte("EBOK")
		kfrec0 = addExth(kfrec0, exthCDEType, s.cdetype)
		if len(cdekey) == 0 {
			kfrec0 = addExth(kfrec0, exthCDEContentKey, s.cdekey)
		}
	} else {
		s.cdetype = []byte("PDOC")
	}
	s.result = writeSection(result, kf8, kfrec0)

	s.processPageData(pdata)
}

func (s *Splitter) produceKF8(data []byte, u []byte, nonPersonal, forceASIN bool) {

	rec0 := readSection(data, 0)
	if a := getInt32(rec0, mobiVersion); a == 8 {
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
	if kf8 < 0 || len(kfrec0) == 0 {
		return
	}

	asin := readExth(rec0, exthASIN)
	if len(asin) == 0 {
		s.asin = u
	} else {
		s.asin = asin[0]
	}

	// save ACR
	const alphabet = `- ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789`
	s.acr = bytes.Map(func(sym rune) rune {
		if sym == 0 {
			return -1
		}
		if strings.ContainsRune(alphabet, sym) {
			return sym
		}
		return '_'
	}, data[0:32])

	// store data for page map
	var pdata []byte
	srcs, numSrcs := getInt32(rec0, firstNonText), getInt16(data, numberOfPdbRecords)
	if srcs >= 0 && numSrcs > 0 {
		for i := srcs; i < numSrcs; i++ {
			d := readSection(data, i)
			if bytes.Equal(d[0:4], []byte("PAGE")) {
				pdata = d
			}
		}
	}

	firstimage, lastimage := getInt32(rec0, firstRescRecord), getInt16(rec0, lastContentIndex)
	if lastimage < 0 {
		// find the lowest of the next sections
		for _, ofs := range []int{fcisIndex, flisIndex, datpIndex, huffTableOffset} {
			n := getInt32(rec0, ofs)
			if n > 0 && n < lastimage {
				lastimage = n - 1
			}
		}
	}

	result := deleteSectionRange(data, 0, kf8-1)
	target := getInt32(kfrec0, firstRescRecord)
	result = insertSectionRange(data, firstimage, lastimage, result, target)
	kfrec0 = readSection(result, 0)

	// Only keep the correct Start Reading offset, KG 2.5 carries over the one from the mobi7 part, which then
	// points at garbage in the mobi8 part, and confuses FW 3.4
	kf8starts := readExth(kfrec0, exthStartReading)
	kf8count := len(kf8starts)
	for kf8count > 1 {
		kf8count--
		kfrec0 = delExth(kfrec0, exthStartReading)
	}

	// update the EXTH 125 KF8 Count of Images/Fonts/Resources
	kfrec0 = writeExth(kfrec0, 125, putInt32(nil, 0, lastimage-firstimage+1))

	// need to reset flags stored in 0x80-0x83
	// old mobi with exth: 0x50, mobi7 part with exth: 0x1850, mobi8 part with exth: 0x1050
	// standalone mobi8 with exth: 0x0050
	// Bit Flags
	// 0x1000 = Bit 12 indicates if embedded fonts are used or not
	// 0x0800 = means this Header points to *shared* images/resource/fonts ??
	// 0x0080 = unknown new flag, why is this now being set by Kindlegen 2.8?
	// 0x0040 = exth exists
	// 0x0010 = Not sure but this is always set so far
	fval := binary.BigEndian.Uint32(kfrec0[0x80:])
	fval &= 0x1FFF
	fval |= 0x0800

	var buf bytes.Buffer
	buf.Write(kfrec0[:0x80])
	binary.Write(&buf, binary.BigEndian, fval)
	buf.Write(kfrec0[0x84:])
	kfrec0 = buf.Bytes()

	// properly update other index pointers that have been shifted by the insertion of images
	for _, ofs := range []int{kf8FdstIndex, fcisIndex, flisIndex, datpIndex, huffTableOffset} {
		n := getInt32(kfrec0, ofs)
		if n >= 0 {
			putInt32(kfrec0, ofs, n+lastimage-firstimage+1)
		}
	}

	exthCover := readExth(kfrec0, exthCoverOffset)
	coverIndex := -1
	if len(exthCover) > 0 {
		coverIndex = getInt32(exthCover[0], 0)
		coverIndex += target
	}

	exthThumb := readExth(kfrec0, exthThumbOffset)
	thumbIndex := -1
	if len(exthThumb) > 0 {
		thumbIndex = getInt32(exthThumb[0], 0)
		thumbIndex += target
	}

	if coverIndex >= 0 {
		if thumbIndex >= 0 {
			// make sure embedded thumbnail has the right size
			coverImage := readSection(data, coverIndex)
			if img, _, err := image.Decode(bytes.NewReader(coverImage)); err == nil {
				if thumb := imaging.Thumbnail(img, 330, 470, imaging.Lanczos); thumb != nil {
					var buf = new(bytes.Buffer)
					if err := imaging.Encode(buf, thumb, imaging.JPEG, imaging.JPEGQuality(75)); err != nil {
						s.log.Error("Unable to encode processed thumbnail, skipping", zap.Error(err))
					} else {
						var jfifAdded bool
						buf, jfifAdded = SetJpegDPI(buf, DpiPxPerInch, 300, 300)
						if jfifAdded {
							s.log.Debug("Inserting JFIF APP0 marker segment into azw3 thumbnail")
						}
						result = writeSection(result, thumbIndex, buf.Bytes())
					}
				}
			}
		} else {
			// old trick, set thumbnail to the cover image, use as a last resort
			kfrec0 = addExth(kfrec0, exthThumbOffset, exthCover[0])
			thumbIndex = coverIndex
		}
		exthrec := readExth(kfrec0, exthThumbnailURI)
		if len(exthrec) > 0 {
			kfrec0 = delExth(kfrec0, exthThumbnailURI)
		}

		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(thumbIndex-target))

		var uri bytes.Buffer
		uri.WriteString("kindle:embed:")
		uri.Write(convertToRadix32(hex.EncodeToString(buf), 4))

		kfrec0 = addExth(kfrec0, exthThumbnailURI, uri.Bytes())
	}

	if nonPersonal {
		s.cdetype = []byte("EBOK")
	} else {
		s.cdetype = []byte("PDOC")
	}
	kfrec0 = addExth(kfrec0, exthCDEType, s.cdetype)

	cdetype := readExth(kfrec0, exthCDEType)
	if len(cdetype) > 0 {
		s.cdetype = cdetype[0]
	}
	cdekey := readExth(kfrec0, exthCDEContentKey)
	if len(cdekey) == 0 {
		s.cdekey = u
		kfrec0 = addExth(kfrec0, exthCDEContentKey, s.cdekey)
	} else {
		s.cdekey = cdekey[0]
	}
	if forceASIN {
		asin := readExth(kfrec0, exthASIN)
		if len(asin) == 0 {
			kfrec0 = addExth(kfrec0, exthASIN, s.cdekey)
		}
	}
	s.result = writeSection(result, 0, kfrec0)

	s.processPageData(pdata)
}

func (s *Splitter) processPageData(data []byte) {

	if len(data) == 0 {
		return
	}

	// NOTE: In some cases kindlegen puts additional word in the header, which looks like length of the offset map.
	// So far this only happens during "transfer" on foreign epub files. We will ignore it for now, but have to
	// account for it to avoid panic. KindleUnpack does not seem to know about it yet.
	ver := getInt16(data, 0x0A)
	revlen := getInt32(data, 0x10+(ver-1)*4)

	// skip over header, revision string length data, and revision string
	ofs := 0x14 + (ver-1)*4 + revlen
	pmlen, pmnn, pmbits := getInt16(data, ofs+2), getInt16(data, ofs+4), getInt16(data, ofs+6)

	pmstr, pmoff := data[ofs+8:ofs+8+pmlen], data[ofs+8+pmlen:]

	var pageOffsets []int
	wordRead, wordSize := getInt32, 4
	if pmbits == 16 {
		wordRead, wordSize = getInt16, 2
	}
	for i, ofs := 0, 0; i < pmnn; i, ofs = i+1, ofs+wordSize {
		od := wordRead(pmoff, ofs)
		pageOffsets = append(pageOffsets, od)
	}

	var pm struct {
		Description string `json:"description"`
		Pagemap     string `json:"pageMap"`
	}
	if err := json.Unmarshal(pmstr, &pm); err != nil {
		s.log.Warn("Unable to parse page map data, ignoring", zap.Error(err))
		return
	}

	asin := s.cdekey
	if len(asin) == 0 {
		asin = s.asin
	}

	var contentHeader string
	if s.combo {
		contentHeader = fmt.Sprintf(`{"contentGuid":"%s","asin":"%s","cdeType":"%s","fileRevisionId":"1"}`,
			s.contentGUID,
			string(asin),
			string(s.cdetype),
		)
	} else {
		contentHeader = fmt.Sprintf(`{"contentGuid":"%s","asin":"%s","cdeType":"%s","format":"MOBI_8","fileRevisionId":"1","acr":"%s"}`,
			s.contentGUID,
			string(asin),
			string(s.cdetype),
			string(s.acr),
		)
	}
	pageHeader := fmt.Sprintf(`{"asin":"%s","pageMap":"%s"}`, string(asin), pm.Pagemap)

	var apnx bytes.Buffer
	binary.Write(&apnx, binary.BigEndian, uint16(1))
	binary.Write(&apnx, binary.BigEndian, uint16(1))
	binary.Write(&apnx, binary.BigEndian, uint32(12+len(contentHeader)))
	binary.Write(&apnx, binary.BigEndian, uint32(len(contentHeader)))
	apnx.WriteString(contentHeader)
	binary.Write(&apnx, binary.BigEndian, uint16(1))
	binary.Write(&apnx, binary.BigEndian, uint16(len(pageHeader)))
	binary.Write(&apnx, binary.BigEndian, uint16(pmnn))
	binary.Write(&apnx, binary.BigEndian, uint16(32))
	apnx.WriteString(pageHeader)
	for _, ofs := range pageOffsets {
		binary.Write(&apnx, binary.BigEndian, uint32(ofs))
	}
	s.pagedata = apnx.Bytes()
}
