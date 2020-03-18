// Package processor does actual work.
//nolint:goconst
package processor

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"math/rand"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asaskevich/govalidator"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"gopkg.in/gomail.v2"

	"github.com/rupor-github/fb2converter/config"
	"github.com/rupor-github/fb2converter/etree"
	"github.com/rupor-github/fb2converter/state"
)

// InputFmt defines type of input we are processing.
type InputFmt int

// Supported formats
const (
	InFb2 InputFmt = iota
	InEpub
)

// Various directories used across the program
const (
	DirContent    = "OEBPS"
	DirMata       = "META-INF"
	DirImages     = "images"
	DirFonts      = "fonts"
	DirVignettes  = "vignettes"
	DirProfile    = "profiles"
	DirHyphenator = "dictionaries"
	DirResources  = "resources"
	DirSentences  = "sentences"
)

// will be used to derive UUIDs from non-parsable book ID
var nameSpaceFB2 = uuid.MustParse("09aa0c17-ca72-42d3-afef-75911e5d7646")

// Processor state.
type Processor struct {
	// what kind of processing is expected
	kind InputFmt
	// input parameters
	src string
	dst string
	// parameters translated to internal types
	nodirs         bool
	stk            bool
	overwrite      bool
	format         OutputFmt
	notesMode      NotesFmt
	tocPlacement   TOCPlacement
	tocType        TOCType
	kindlePageMap  APNXGeneration
	stampPlacement StampPlacement
	coverResize    CoverProcessing
	// working directory
	tmpDir string
	// input document
	doc *etree.Document
	// parsing state and conversion results
	Book     *Book
	notFound *binImage
	// program environment
	env             *state.LocalEnv
	speechTransform *config.Transformation
	dashTransform   *config.Transformation
	metaOverwrite   *config.MetaInfo
	kindlegenPath   string
}

// NewFB2 creates FB2 book processor and prepares necessary temporary directories.
func NewFB2(r io.Reader, unknownEncoding bool, src, dst string, nodirs, stk, overwrite bool, format OutputFmt, env *state.LocalEnv) (*Processor, error) {

	kindle := format == OAzw3 || format == OMobi

	u, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate UUID")
	}

	notes := ParseNotesString(env.Cfg.Doc.Notes.Mode)
	if notes == UnsupportedNotesFmt {
		env.Log.Warn("Unknown notes mode requested, switching to default", zap.String("mode", env.Cfg.Doc.Notes.Mode))
		notes = NDefault
	}
	if notes != NFloat && notes != NFloatOld && notes != NFloatNew && env.Cfg.Doc.Notes.Renumber {
		env.Log.Warn("Notes can be renumbered in floating modes only, ignoring", zap.String("mode", env.Cfg.Doc.Notes.Mode))
	}
	toct := ParseTOCTypeString(env.Cfg.Doc.TOC.Type)
	if toct == UnsupportedTOCType {
		env.Log.Warn("Unknown TOC type requested, switching to normal", zap.String("type", env.Cfg.Doc.TOC.Type))
		toct = TOCTypeNormal
	}
	place := ParseTOCPlacementString(env.Cfg.Doc.TOC.Placement)
	if place == UnsupportedTOCPlacement {
		env.Log.Warn("Unknown TOC page placement requested, turning off generation", zap.String("placement", env.Cfg.Doc.TOC.Placement))
		place = TOCNone
	}
	var apnx APNXGeneration
	if kindle {
		if stk && format == OMobi && env.Cfg.SMTPConfig.IsValid() && env.Cfg.SMTPConfig.DeleteOnSuccess {
			// Do not create pagemap - we do not need it
			apnx = APNXNone
		} else {
			apnx = ParseAPNXGenerationSring(env.Cfg.Doc.Kindlegen.PageMap)
			if apnx == UnsupportedAPNXGeneration {
				env.Log.Warn("Unknown APNX generation option requested, turning off", zap.String("apnx", env.Cfg.Doc.Kindlegen.PageMap))
				apnx = APNXNone
			}
		}
	}
	var stamp StampPlacement
	if len(env.Cfg.Doc.Cover.Placement) > 0 {
		stamp = ParseStampPlacementString(env.Cfg.Doc.Cover.Placement)
		if stamp == UnsupportedStampPlacement {
			env.Log.Warn("Unknown stamp placement requested, using default (none - if book has cover, middle - otherwise)", zap.String("placement", env.Cfg.Doc.Cover.Placement))
		}
	}
	var resize CoverProcessing
	if len(env.Cfg.Doc.Cover.Resize) > 0 {
		resize = ParseCoverProcessingString(env.Cfg.Doc.Cover.Resize)
		if resize == UnsupportedCoverProcessing {
			env.Log.Warn("Unknown cover resizing mode requested, using default", zap.String("resize", env.Cfg.Doc.Cover.Resize))
			resize = CoverNone
		}
	}

	p := &Processor{
		kind:            InFb2,
		src:             src,
		dst:             dst,
		nodirs:          nodirs,
		stk:             stk,
		overwrite:       overwrite,
		format:          format,
		notesMode:       notes,
		tocType:         toct,
		tocPlacement:    place,
		kindlePageMap:   apnx,
		stampPlacement:  stamp,
		coverResize:     resize,
		doc:             etree.NewDocument(),
		Book:            NewBook(u, filepath.Base(src)),
		env:             env,
		speechTransform: env.Cfg.GetTransformation("speech"),
		dashTransform:   env.Cfg.GetTransformation("dashes"),
		metaOverwrite:   env.Cfg.GetOverwrite(src),
	}
	p.doc.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}

	if kindle {
		// Fail early
		if p.kindlegenPath, err = env.Cfg.GetKindlegenPath(); err != nil {
			return nil, err
		}
	}

	// sanity checking
	if p.speechTransform != nil && len(p.speechTransform.To) == 0 {
		env.Log.Warn("Invalid direct speech transformation, ignoring")
		p.speechTransform = nil
	}
	if p.dashTransform != nil && len(p.dashTransform.To) == 0 {
		env.Log.Warn("Invalid dash transformation, ignoring")
		p.dashTransform = nil
	}
	if p.dashTransform != nil {
		sym, _ := utf8.DecodeRuneInString(p.dashTransform.To)
		p.dashTransform.To = string(sym)
	}

	// re-route temporary directory for debugging
	if env.Debug {
		wd, err := os.Getwd()
		if err != nil {
			return nil, errors.Wrap(err, "unable to get working directory")
		}
		tmpd := filepath.Join(wd, "fb2c_deb")
		if err = os.MkdirAll(tmpd, 0700); err != nil {
			return nil, errors.Wrap(err, "unable to create debug directory")
		}
		t := time.Now()
		ulid, err := ulid.New(ulid.Timestamp(t), ulid.Monotonic(rand.New(rand.NewSource(t.UnixNano())), 0))
		if err != nil {
			return nil, errors.Wrap(err, "unable to allocate ULID")
		}
		p.tmpDir = filepath.Join(tmpd, ulid.String()+"_"+filepath.Base(src))
		if err = os.MkdirAll(p.tmpDir, 0700); err != nil {
			return nil, errors.Wrap(err, "unable to create temporary directory")
		}
	} else {
		p.tmpDir, err = ioutil.TempDir("", "fb2c-")
		if err != nil {
			return nil, errors.Wrap(err, "unable to create temporary directory")
		}
	}

	if unknownEncoding {
		// input file had no BOM mark - most likely was not Unicode
		p.doc.ReadSettings = etree.ReadSettings{
			CharsetReader: charset.NewReaderLabel,
		}
	}

	// Read and parse fb2
	if _, err := p.doc.ReadFrom(r); err != nil {
		return nil, errors.Wrap(err, "unable to parse FB2")
	}

	// Clean document
	p.doc.Indent(etree.NoIndent)

	// Save parsed document back to file (pretty-printed) for debugging
	if p.env.Debug {
		doc := p.doc.Copy()
		doc.IndentTabs()
		if err := doc.WriteToFile(filepath.Join(p.tmpDir, filepath.Base(src))); err != nil {
			return nil, errors.Wrap(err, "unable to write XML")
		}
	}

	// we are ready to convert document
	return p, nil
}

// NewEPUB creates EPUB book processor and prepares necessary temporary directories.
func NewEPUB(r io.Reader, src, dst string, nodirs, stk, overwrite bool, format OutputFmt, env *state.LocalEnv) (*Processor, error) {

	var err error

	var apnx APNXGeneration
	if format == OAzw3 || format == OMobi {
		if stk && format == OMobi && env.Cfg.SMTPConfig.IsValid() && env.Cfg.SMTPConfig.DeleteOnSuccess {
			// Do not create pagemap - we do not need it
			apnx = APNXNone
		} else {
			apnx = ParseAPNXGenerationSring(env.Cfg.Doc.Kindlegen.PageMap)
			if apnx == UnsupportedAPNXGeneration {
				env.Log.Warn("Unknown APNX generation option requested, turning off", zap.String("apnx", env.Cfg.Doc.Kindlegen.PageMap))
				apnx = APNXNone
			}
		}
	}

	p := &Processor{
		kind:          InEpub,
		src:           src,
		dst:           dst,
		nodirs:        nodirs,
		stk:           stk,
		kindlePageMap: apnx,
		overwrite:     overwrite,
		format:        format,
		env:           env,
	}

	// Fail early
	if p.kindlegenPath, err = env.Cfg.GetKindlegenPath(); err != nil {
		return nil, err
	}

	// re-route temporary directory for debugging
	if env.Debug {
		wd, err := os.Getwd()
		if err != nil {
			return nil, errors.Wrap(err, "unable to get working directory")
		}
		tmpd := filepath.Join(wd, "fb2c_deb")
		if err = os.MkdirAll(tmpd, 0700); err != nil {
			return nil, errors.Wrap(err, "unable to create debug directory")
		}
		t := time.Now()
		ulid, err := ulid.New(ulid.Timestamp(t), ulid.Monotonic(rand.New(rand.NewSource(t.UnixNano())), 0))
		if err != nil {
			return nil, errors.Wrap(err, "unable to allocate ULID")
		}
		p.tmpDir = filepath.Join(tmpd, ulid.String()+"_"+filepath.Base(src))
		if err = os.MkdirAll(p.tmpDir, 0700); err != nil {
			return nil, errors.Wrap(err, "unable to create temporary directory")
		}
	} else {
		p.tmpDir, err = ioutil.TempDir("", "fb2c-")
		if err != nil {
			return nil, errors.Wrap(err, "unable to create temporary directory")
		}
	}

	// copy source file to temporary directory - when we decide what else to do with EPUBs this will be very handy

	if destination, err := os.Create(filepath.Join(p.tmpDir, filepath.Base(src))); err == nil {
		defer destination.Close()
		if _, err := io.Copy(destination, r); err != nil {
			return nil, errors.Wrap(err, "unable to copy source")
		}
	} else {
		return nil, errors.Wrap(err, "unable to copy source")
	}

	// we are ready to convert document
	return p, nil
}

// Process does all the work.
func (p *Processor) Process() error {

	if p.kind == InEpub {
		// later we may decide to clean epub, massage its stylesheet, etc.
		return nil
	}

	// Debugging
	defer func() {
		if p.env.Debug && p.kind == InFb2 {
			// Dump processed book for debugging
			bname := filepath.Base(p.src)
			dump, err := os.Create(filepath.Join(p.tmpDir, strings.TrimSuffix(bname, filepath.Ext(p.src))+"-dump.gz"))
			if err != nil {
				p.env.Log.Debug("Unable to create file to dump internal state to", zap.Error(err))
				return
			}
			defer dump.Close()

			zdump, err := gzip.NewWriterLevel(dump, gzip.BestSpeed)
			if err != nil {
				p.env.Log.Debug("Unable to compress internal state dump", zap.Error(err))
			}
			defer zdump.Close()

			zdump.Name = bname
			zdump.Comment = "fb2c debug dump"
			p.Book.Dump(zdump)
		}
	}()

	// Processing - order of steps and their presence are important as information and context
	// being built and accumulated...

	if err := p.processNotes(); err != nil {
		return err
	}
	if err := p.processBinaries(); err != nil {
		return err
	}
	if err := p.processDescription(); err != nil {
		return err
	}
	if err := p.processBodies(); err != nil {
		return err
	}
	if err := p.processLinks(); err != nil {
		return err
	}
	if err := p.processImages(); err != nil {
		return err
	}
	if err := p.generateTOCPage(); err != nil {
		return err
	}
	if err := p.generateCover(); err != nil {
		return err
	}
	if err := p.generateNCX(); err != nil {
		return err
	}
	if err := p.prepareStylesheet(); err != nil {
		return err
	}
	if err := p.generatePagemap(); err != nil {
		return err
	}
	if err := p.generateOPF(); err != nil {
		return err
	}
	if err := p.generateMeta(); err != nil {
		return err
	}
	return p.KepubifyXHTML()
}

// Save makes the conversion results permanent by storing everything properly and cleaning temporary artifacts.
func (p *Processor) Save() (string, error) {

	start := time.Now()
	p.env.Log.Debug("Saving content - starting",
		zap.String("tmp", p.tmpDir),
		zap.String("content", DirContent),
	)
	defer func(start time.Time) {
		p.env.Log.Debug("Saving content - done", zap.Duration("elapsed", time.Since(start)))
	}(start)

	if p.kind == InFb2 {
		if err := p.Book.flushData(p.tmpDir); err != nil {
			return "", err
		}
		if err := p.Book.flushVignettes(p.tmpDir); err != nil {
			return "", err
		}
		if err := p.Book.flushImages(p.tmpDir); err != nil {
			return "", err
		}
		if err := p.Book.flushXHTML(p.tmpDir); err != nil {
			return "", err
		}
		if err := p.Book.flushMeta(p.tmpDir); err != nil {
			return "", err
		}
	}

	fname := p.prepareOutputName()

	var err error
	switch p.format {
	case OEpub:
		err = p.FinalizeEPUB(fname)
	case OKepub:
		err = p.FinalizeKEPUB(fname)
	case OMobi:
		err = p.FinalizeMOBI(fname)
	case OAzw3:
		err = p.FinalizeAZW3(fname)
	}
	return fname, err
}

// SendToKindle will mail converted file to specified address and remove file if requested.
func (p *Processor) SendToKindle(fname string) error {

	if !p.stk || p.format != OMobi || len(fname) == 0 {
		return nil
	}

	if !p.env.Cfg.SMTPConfig.IsValid() {
		p.env.Log.Warn("Configuration for Send To Kindle is incorrect, skipping", zap.Any("configuration", p.env.Cfg.SMTPConfig))
		return nil
	}

	start := time.Now()
	p.env.Log.Debug("Sending content to Kindle - starting",
		zap.String("from", p.env.Cfg.SMTPConfig.From),
		zap.String("to", p.env.Cfg.SMTPConfig.To),
		zap.String("file", fname),
	)
	defer func(start time.Time) {
		p.env.Log.Debug("Sending content to Kindle - done", zap.Duration("elapsed", time.Since(start)))
	}(start)

	m := gomail.NewMessage()
	m.SetHeader("From", p.env.Cfg.SMTPConfig.From)
	m.SetAddressHeader("To", p.env.Cfg.SMTPConfig.To, "kindle")
	m.SetHeader("Subject", "Sent to Kindle")
	m.SetBody("text/plain", "This email has been automatically sent by fb2converter tool")
	m.Attach(fname)

	d := gomail.NewDialer(p.env.Cfg.SMTPConfig.Server, p.env.Cfg.SMTPConfig.Port, p.env.Cfg.SMTPConfig.User, p.env.Cfg.SMTPConfig.Password)

	if err := d.DialAndSend(m); err != nil {
		return errors.Wrap(err, "SentToKindle failed")
	}

	if p.env.Cfg.SMTPConfig.DeleteOnSuccess {
		p.env.Log.Debug("Deleting after send", zap.String("location", fname))
		if err := os.Remove(fname); err != nil {
			p.env.Log.Warn("Unable to delete after send", zap.String("location", fname), zap.Error(err))
		}
		if !p.nodirs {
			// remove all empty directories in the path following p.dst
			for outDir := filepath.Dir(fname); outDir != p.dst; outDir = filepath.Dir(outDir) {
				if err := os.Remove(outDir); err != nil {
					p.env.Log.Warn("Unable to delete after send", zap.String("location", outDir), zap.Error(err))
				}
			}
		}
	}
	return nil
}

// Clean removes temporary files left after processing.
func (p *Processor) Clean() error {
	if p.env.Debug {
		// Leave temporary files intact
		return nil
	}
	p.env.Log.Debug("Cleaning", zap.String("location", p.tmpDir))
	return os.RemoveAll(p.tmpDir)
}

// prepareOutputName generates output file name.
func (p *Processor) prepareOutputName() string {

	var outDir string
	if !p.nodirs {
		outDir = filepath.Dir(p.src)
	}
	outDir = filepath.Join(p.dst, outDir)

	name := strings.TrimSuffix(filepath.Base(p.src), filepath.Ext(p.src))
	if p.env.Cfg.Doc.FileNameTransliterate {
		name = slug.Make(name)
	}
	outFile := config.CleanFileName(name) + "." + p.format.String()
	if p.format == OKepub {
		outFile += "." + OEpub.String()
	}

	if p.kind == InFb2 && len(p.env.Cfg.Doc.FileNameFormat) > 0 {

		insertDir := func(dirs []string, dir string) []string {
			dirs = append(dirs, "")
			copy(dirs[1:], dirs[0:])
			dirs[0] = dir
			return dirs
		}

		name = filepath.FromSlash(ReplaceKeywords(p.env.Cfg.Doc.FileNameFormat, CreateFileNameKeywordsMap(p.Book, p.env.Cfg.Doc.AuthorFormatFileName, p.env.Cfg.Doc.SeqNumPos)))
		if len(name) > 0 {
			first := true
			dirs := make([]string, 0, 16)
			for head, tail := filepath.Split(strings.TrimSuffix(name, string(os.PathSeparator))); ; head, tail = filepath.Split(strings.TrimSuffix(head, string(os.PathSeparator))) {
				if first {
					if p.env.Cfg.Doc.FileNameTransliterate {
						tail = slug.Make(tail)
					}
					outFile = config.CleanFileName(tail) + "." + p.format.String()
					if p.format == OKepub {
						outFile += "." + OEpub.String()
					}
					first = false
				} else {
					if p.env.Cfg.Doc.FileNameTransliterate {
						tail = slug.Make(tail)
					}
					dirs = insertDir(dirs, config.CleanFileName(tail))
				}
				if len(head) == 0 {
					break
				}
			}
			dirs = insertDir(dirs, outDir)
			outDir = filepath.Join(dirs...)
		}
	}
	return filepath.Join(outDir, outFile)
}

// processDescription processes book description element.
func (p *Processor) processDescription() error {

	start := time.Now()
	p.env.Log.Debug("Parsing description - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Parsing description - done",
			zap.Duration("elapsed", time.Since(start)),
			zap.Stringer("id", p.Book.ID),
			zap.String("asin", p.Book.ASIN),
			zap.String("title", p.Book.Title),
			zap.Stringer("lang", p.Book.Lang),
			zap.String("cover", p.Book.Cover),
			zap.Strings("genres", p.Book.Genres),
			zap.String("authors", p.Book.BookAuthors(p.env.Cfg.Doc.AuthorFormat, false)),
			zap.String("sequence", p.Book.SeqName),
			zap.Int("sequence number", p.Book.SeqNum),
			zap.String("date", p.Book.Date),
		)
	}(start)

	for _, desc := range p.doc.FindElements("./FictionBook/description") {

		if info := desc.SelectElement("document-info"); info != nil {
			if id := info.SelectElement("id"); id != nil {
				text := strings.TrimSpace(id.Text())
				if u, err := uuid.Parse(text); err == nil {
					p.Book.ID = u
				} else {
					p.env.Log.Debug("Unable to parse book id, deriving new", zap.String("id", text), zap.Error(err))
					p.Book.ID = uuid.NewSHA1(nameSpaceFB2, []byte(text))
				}
			}
		}
		if info := desc.SelectElement("title-info"); info != nil {
			if e := info.SelectElement("book-title"); e != nil {
				if t := strings.TrimSpace(e.Text()); len(t) > 0 {
					p.Book.Title = t
				}
			}
			if e := info.SelectElement("lang"); e != nil {
				if l := strings.TrimSpace(e.Text()); len(l) > 0 {
					t, err := language.Parse(l)
					if err != nil {
						// last resort - try names directly
						for _, st := range display.Supported.Tags() {
							if strings.EqualFold(display.Self.Name(st), l) {
								t = st
								err = nil
								break
							}
						}
						if err != nil {
							return err
						}
					}
					p.Book.Lang = t
					if p.env.Cfg.Doc.Hyphenate {
						p.Book.hyph = newHyph(t, p.env.Log)
					}
					if p.format == OKepub {
						p.Book.tokenizer = newTokenizer(t, p.env.Log)
					}
				}
			}
			if e := info.SelectElement("coverpage"); e != nil {
				if i := e.SelectElement("image"); i != nil {
					c := getAttrValue(i, "href")
					if len(c) > 0 {
						if u, err := url.Parse(c); err != nil {
							p.env.Log.Warn("Unable to parse cover image href", zap.String("href", c), zap.Error(err))
						} else {
							p.Book.Cover = u.Fragment
						}
					}
				}
			}
			for _, e := range info.SelectElements("genre") {
				if g := strings.TrimSpace(e.Text()); len(g) > 0 {
					p.Book.Genres = append(p.Book.Genres, g)
				}
			}
			for _, e := range info.SelectElements("author") {
				var (
					an       = new(config.AuthorName)
					notEmpty bool
				)
				if n := e.SelectElement("first-name"); n != nil {
					if f := strings.TrimSpace(n.Text()); len(f) > 0 {
						an.First = f
						notEmpty = true
					}
				}
				if n := e.SelectElement("middle-name"); n != nil {
					if m := strings.TrimSpace(n.Text()); len(m) > 0 {
						an.Middle = m
						notEmpty = true
					}
				}
				if n := e.SelectElement("last-name"); n != nil {
					if l := strings.TrimSpace(n.Text()); len(l) > 0 {
						an.Last = l
						notEmpty = true
					}
				}
				if notEmpty {
					p.Book.Authors = append(p.Book.Authors, an)
				}
			}
			if e := info.SelectElement("sequence"); e != nil {
				var err error
				p.Book.SeqName = getAttrValue(e, "name")
				num := getAttrValue(e, "number")
				if len(num) > 0 {
					if !govalidator.IsNumeric(num) {
						p.env.Log.Warn("Sequence number is not an integer, ignoring", zap.String("xml", getXMLFragmentFromElement(e)))
					} else {
						p.Book.SeqNum, err = strconv.Atoi(num)
						if err != nil {
							p.env.Log.Warn("Unable to parse sequence number, ignoring", zap.String("number", getAttrValue(e, "number")), zap.Error(err))
						}
					}
				}
			}
			if e := info.SelectElement("annotation"); e != nil {
				p.Book.Annotation = getTextFragment(e)
				if p.env.Cfg.Doc.Annotation.Create {
					to, f := p.ctx().createXHTML("annotation")
					inner := to.AddNext("div", attr("class", "annotation"))
					inner.AddNext("div", attr("class", "h1")).SetText(p.env.Cfg.Doc.Annotation.Title)
					if err := p.transfer(e, inner, "div"); err != nil {
						p.env.Log.Warn("Unable to parse annotation", zap.String("path", e.GetPath()), zap.Error(err))
					} else {
						p.Book.Files = append(p.Book.Files, f)
						if p.env.Cfg.Doc.Annotation.AddToToc {
							tocRefID := fmt.Sprintf("tocref%d", p.ctx().tocIndex)
							inner.CreateAttr("id", tocRefID)
							p.Book.TOC = append(p.Book.TOC, &tocEntry{
								ref:      p.ctx().fname + "#" + tocRefID,
								title:    p.env.Cfg.Doc.Annotation.Title,
								level:    p.ctx().header,
								bodyName: p.ctx().bodyName,
							})
							p.ctx().tocIndex++
						}
					}
				}
			}
			if e := info.SelectElement("date"); e != nil {
				p.Book.Date = getTextFragment(e)
			}
		}
	}

	// Let's see if we need to correct any meta information - always comes last
	if p.metaOverwrite == nil {
		return nil
	}

	if len(p.metaOverwrite.ID) > 0 {
		if u, err := uuid.Parse(strings.TrimSpace(p.metaOverwrite.ID)); err == nil {
			p.Book.ID = u
			p.env.Log.Info("Meta overwrite", zap.Stringer("id", p.Book.ID))
		}
	}
	if len(p.metaOverwrite.ASIN) == 10 && govalidator.IsAlphanumeric(p.metaOverwrite.ASIN) {
		p.Book.ASIN = p.metaOverwrite.ASIN
		p.env.Log.Info("Meta overwrite", zap.String("asin", p.Book.ASIN))
	}
	title := strings.TrimSpace(p.metaOverwrite.Title)
	if len(title) > 0 {
		p.Book.Title = title
		p.env.Log.Info("Meta overwrite", zap.String("title", p.Book.Title))
	}
	if len(p.metaOverwrite.Lang) > 0 {
		if l := strings.TrimSpace(p.metaOverwrite.Lang); len(l) > 0 {
			if t, err := language.Parse(l); err == nil {
				p.Book.Lang = t
				p.env.Log.Info("Meta overwrite", zap.Stringer("lang", p.Book.Lang))
				if p.env.Cfg.Doc.Hyphenate {
					p.Book.hyph = newHyph(t, p.env.Log)
				}
			}
		}
	}
	var genres []string
	for _, e := range p.metaOverwrite.Genres {
		if g := strings.TrimSpace(e); len(g) > 0 {
			genres = append(genres, g)
		}
	}
	if len(genres) > 0 {
		p.Book.Genres = genres
		p.env.Log.Info("Meta overwrite", zap.Strings("genres", p.Book.Genres))
	}
	if len(p.metaOverwrite.Authors) > 0 {
		p.Book.Authors = append([]*config.AuthorName{}, p.metaOverwrite.Authors...)
		p.env.Log.Info("Meta overwrite", zap.String("authors", p.Book.BookAuthors(p.env.Cfg.Doc.AuthorFormat, false)))
	}
	seq := strings.TrimSpace(p.metaOverwrite.SeqName)
	if len(seq) > 0 {
		p.Book.SeqName = seq
		p.env.Log.Info("Meta overwrite", zap.String("sequence", p.Book.SeqName))
	}
	if p.metaOverwrite.SeqNum > 0 {
		p.Book.SeqNum = p.metaOverwrite.SeqNum
		p.env.Log.Info("Meta overwrite", zap.Int("sequence number", p.Book.SeqNum))
	}
	date := strings.TrimSpace(p.metaOverwrite.Date)
	if len(date) > 0 {
		p.Book.Date = date
		p.env.Log.Info("Meta overwrite", zap.String("date", p.Book.Date))
	}
	return nil
}

// processBodies processes book bodies, including main one.
func (p *Processor) processBodies() error {

	start := time.Now()
	p.env.Log.Debug("Parsing bodies - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Parsing bodies - done",
			zap.Duration("elapsed", time.Since(start)),
		)
	}(start)

	for i, body := range p.doc.FindElements("./FictionBook/body") {
		if err := p.processBody(i, body); err != nil {
			return err
		}
	}

	return nil
}

func (p *Processor) parseNoteSectionElement(el *etree.Element, name string, notesPerBody map[string]int) {

	switch {
	case el.Tag == "title":
		// Sometimes note section has separate title - we want to use it in TOC
		t := SanitizeTitle(getTextFragment(el))
		if len(t) > 0 {
			if _, ok := p.Book.NoteBodyTitles[name]; ok {
				// second title section in the same notes body - ignore for now
				p.env.Log.Debug("Attempt to stick different notes type/title into single document body. Ignoring...", zap.String("title", t))
				return
			}
			ctx := p.ctxPush()
			ctx.inHeader = true
			if err := p.transfer(el, &ctx.out.Element, "div", "h0"); err != nil {
				p.env.Log.Warn("Unable to parse notes body title", zap.String("path", el.GetPath()), zap.Error(err))
			}
			ctx.inHeader = false
			p.ctxPop()

			child := ctx.out.FindElement("./*")
			p.Book.NoteBodyTitles[name] = &note{
				title:      t,
				body:       getTextFragment(child),
				bodyName:   name,
				bodyNumber: len(notesPerBody),
				parsed:     child.Copy(),
			}
		}
	case el.Tag == "section" && getAttrValue(el, "id") != "":
		id := getAttrValue(el, "id")
		notesPerBody[name]++
		note := &note{
			number:     notesPerBody[name],
			bodyName:   name,
			bodyNumber: len(notesPerBody),
		}
		for _, c := range el.ChildElements() {
			t := getTextFragment(c)
			if c.Tag == "title" {
				note.title = SanitizeTitle(t)
			} else {
				note.body += t
			}
		}
		p.Book.NotesOrder = append(p.Book.NotesOrder, notelink{id: id, bodyName: name})
		p.Book.Notes[id] = note
	default:
		// Sometimes there are sections inside sections to no end...
		for _, section := range el.ChildElements() {
			p.parseNoteSectionElement(section, name, notesPerBody)
		}
	}
}

// processNotes processes notes bodies. We will need notes when main body is parsed.
func (p *Processor) processNotes() error {

	start := time.Now()
	p.env.Log.Debug("Parsing notes - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Parsing notes - done",
			zap.Duration("elapsed", time.Since(start)),
			zap.Int("body titles", len(p.Book.NoteBodyTitles)),
			zap.Int("notes bodies", p.Book.NotesBodies),
			zap.Int("notes", len(p.Book.NotesOrder)),
		)
	}(start)

	notesPerBody := make(map[string]int)
	for _, el := range p.doc.FindElements("./FictionBook/body[@name]") {
		name := getAttrValue(el, "name")
		if !IsOneOf(name, p.env.Cfg.Doc.Notes.BodyNames) {
			continue
		}
		notesPerBody[name] = 0
		for _, section := range el.ChildElements() {
			p.parseNoteSectionElement(section, name, notesPerBody)
		}
	}
	p.Book.NotesBodies = len(notesPerBody)
	return nil
}

// processBinaries processes book images.
func (p *Processor) processBinaries() error {

	start := time.Now()
	p.env.Log.Debug("Parsing images - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Parsing images - done",
			zap.Duration("elapsed", time.Since(start)),
			zap.Int("images", len(p.Book.Images)),
		)
	}(start)

	for i, el := range p.doc.FindElements("./FictionBook/binary[@id]") {

		id := getAttrValue(el, "id")
		declaredCT := getAttrValue(el, "content-type")

		s := strings.TrimSpace(el.Text())
		// some files are badly formatted
		s = strings.Replace(s, " ", "", -1)
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			// And some have several images staffed together
			// NOTE: I know this is wrong, but short of writing my own base64 decoder this should do...
			const errString = "illegal base64 data at input byte "
			if strings.HasPrefix(err.Error(), errString) {
				i, er := strconv.ParseInt(strings.TrimPrefix(err.Error(), errString), 10, 64)
				if er != nil {
					return errors.Wrapf(err, "unable to decode binary (%s)", id)
				}
				// try to ignore everything after error position
				data, er = base64.StdEncoding.DecodeString(s[0:i])
				if er != nil {
					return errors.Wrapf(err, "unable to decode binary (%s)", id)
				}
			} else {
				return errors.Wrapf(err, "unable to decode binary (%s)", id)
			}
		}

		if strings.HasSuffix(strings.ToLower(declaredCT), "svg") {
			// Special case - do not touch SVG
			p.Book.Images = append(p.Book.Images, &binImage{
				log:     p.env.Log,
				id:      id,
				ct:      "image/svg+xml",
				fname:   fmt.Sprintf("bin%08d.svg", i),
				relpath: filepath.Join(DirContent, DirImages),
				imgType: "svg",
				data:    data,
			})
			continue
		}

		var (
			detectedCT string
			doNotTouch bool
		)

		img, imgType, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			p.env.Log.Warn("Unable to decode image",
				zap.String("id", id),
				zap.String("declared", declaredCT),
				zap.Error(err))

			if !p.env.Cfg.Doc.UseBrokenImages {
				continue
			}

			detectedCT = declaredCT
			doNotTouch = true
		} else {
			detectedCT = mime.TypeByExtension("." + imgType)
		}

		if !strings.EqualFold(declaredCT, detectedCT) {
			p.env.Log.Warn("Declared and detected image types do not match, using detected type",
				zap.String("id", id),
				zap.String("declared", declaredCT),
				zap.String("detected", detectedCT))
		}

		// fill in image info
		b := &binImage{
			log:     p.env.Log,
			id:      id,
			ct:      detectedCT,
			fname:   fmt.Sprintf("bin%08d.%s", i, imgType),
			relpath: filepath.Join(DirContent, DirImages),
			img:     img,
			imgType: imgType,
			data:    data,
		}

		if !doNotTouch {
			// see if any additional processing is requested
			if !isImageSupported(b.imgType) && (p.format == OMobi || p.format == OAzw3) {
				b.flags |= imageKindle
			}
			if p.env.Cfg.Doc.RemovePNGTransparency && imgType == "png" {
				b.flags |= imageOpaquePNG
			}
			if p.env.Cfg.Doc.ImagesScaleFactor > 0 && (imgType == "png" || imgType == "jpeg") {
				b.flags |= imageScale
				b.scaleFactor = p.env.Cfg.Doc.ImagesScaleFactor
			}
		}
		p.Book.Images = append(p.Book.Images, b)
	}
	return nil
}

// processLinks goes over generated documents and makes sure hanging anchors are properly anchored.
func (p *Processor) processLinks() error {

	start := time.Now()
	p.env.Log.Debug("Processing links - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Processing links - done",
			zap.Duration("elapsed", time.Since(start)),
		)
	}(start)

	for _, f := range p.Book.Files {
		if f.doc == nil {
			continue
		}
		for _, a := range f.doc.FindElements("//a[@href]") {
			href := getAttrValue(a, "href")
			if !strings.HasPrefix(href, "#") {
				continue
			}
			if fname, ok := p.Book.LinksLocations[href[1:]]; ok {
				a.CreateAttr("href", fname+href)
			}
		}
	}
	return nil
}

// processImages makes sure that images we use have suitable properties.
func (p *Processor) processImages() error {

	start := time.Now()
	p.env.Log.Debug("Processing images - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Processing images - done",
			zap.Duration("elapsed", time.Since(start)),
		)
	}(start)

	if len(p.Book.Cover) > 0 {
		// some badly formatted fb2 have several covers (LibRusEq - engineers with two left feet) leave only first one
		haveFirstCover, haveExtraCovers := false, false
		for i, b := range p.Book.Images {
			if b.id == p.Book.Cover {
				if haveFirstCover {
					haveExtraCovers = true
					p.Book.Images[i].id = "" // mark for removal
				} else {
					haveFirstCover = true
					// Since we are here anyway - let's see if we need to correct cover information
					if p.metaOverwrite != nil && len(p.metaOverwrite.CoverImage) > 0 {
						var (
							err error
							b   = &binImage{id: b.id, log: p.env.Log, relpath: filepath.Join(DirContent, DirImages)}
						)
						fname := p.metaOverwrite.CoverImage
						if !filepath.IsAbs(fname) {
							fname = filepath.Join(p.env.Cfg.Path, fname)
						}
						if b.data, err = ioutil.ReadFile(fname); err == nil {
							if b.img, b.imgType, err = image.Decode(bytes.NewReader(b.data)); err == nil {
								b.ct = mime.TypeByExtension("." + b.imgType)
								b.fname = strings.TrimSuffix(p.Book.Images[i].fname, filepath.Ext(p.Book.Images[i].fname)) + "." + b.imgType
								p.Book.Images[i] = b
								p.env.Log.Info("Meta overwrite", zap.String("cover", fname))
							}
						}
					}
					// NOTE: We will process cover separately
					b.flags &= ^imageScale
					b.scaleFactor = 0
				}
			}
		}
		if haveExtraCovers {
			p.env.Log.Warn("Removing cover image duplicates, leaving only the first one")
			for i := len(p.Book.Images) - 1; i >= 0; i-- {
				if p.Book.Images[i].id == "" {
					p.Book.Images = append(p.Book.Images[:i], p.Book.Images[i+1:]...)
				}
			}
		}
	} else if p.env.Cfg.Doc.Cover.Default || p.format == OMobi || p.format == OAzw3 {
		// For Kindle we always supply cover image if none is present, for others - only if asked to
		b, err := p.getDefaultCover(len(p.Book.Images))
		if err != nil {
			// not found or cannot be decoded, misconfiguration - stop here
			return err
		}
		p.env.Log.Debug("Providing default cover image")
		p.Book.Cover = b.id
		p.Book.Images = append(p.Book.Images, b)
		if p.stampPlacement == StampNone {
			// default cover always stamped
			p.stampPlacement = StampMiddle
		}
	}
	return nil
}

// shortcuts
func (p *Processor) ctx() *context {
	return p.Book.ctx()
}

func (p *Processor) ctxPush() *context {
	return p.Book.ctxPush()
}

func (p *Processor) ctxPop() *context {
	return p.Book.ctxPop()
}
