package processor

import (
	"fmt"
	"math"
	"mime"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gosimple/slug"
	"go.uber.org/zap"

	"fb2converter/etree"
)

// generateTOCPage creates an HTML page with TOC.
func (p *Processor) generateTOCPage() error {

	if p.tocPlacement == TOCNone || len(p.Book.TOC) == 0 {
		return nil
	}

	p.env.Log.Debug("Generating TOC page - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Generating TOC page - done",
			zap.Duration("elapsed", time.Since(start)),
		)
	}(time.Now())

	to, f := p.ctx().createXHTML("toc", attr("xmlns", `http://www.w3.org/1999/xhtml`))
	f.id = "toc"

	switch p.tocPlacement {
	case TOCBefore:
		// TOC page goes first
		p.Book.Files = append(p.Book.Files, nil)
		copy(p.Book.Files[1:], p.Book.Files[0:])
		p.Book.Files[0] = f
	case TOCAfter:
		p.Book.Files = append(p.Book.Files, f)
	}

	toc := to.AddNext("div", attr("class", "toc"))
	toc.AddNext("div", attr("id", "toc"), attr("class", "h1")).SetText(p.env.Cfg.Doc.TOC.Title)

	for _, te := range p.Book.TOC {
		if te.level.Int() > p.env.Cfg.Doc.TOC.MaxLevel {
			continue
		}
		if len(te.bodyName) > 0 {
			toc.AddNext("div", attr("class", "indent0")).AddNext("a", attr("href", te.ref)).SetText(AllLines(te.title))
			continue
		}
		if te.level.Int() > 0 {
			toc.AddNext("div", attr("class", te.level.String("indent"))).AddNext("a", attr("href", te.ref)).SetText(AllLines(te.title))
			continue
		}
		inner := toc.AddNext("div", attr("class", "indent0")).AddNext("a", attr("href", te.ref))

		if p.env.Cfg.Doc.TOC.BookTitleFromMeta && te.main {
			if p.version == 1 {
				inner.AddNext("span", attr("class", "toc_author")).SetText(p.Book.BookAuthors(p.env.Cfg.Doc.AuthorFormat, false))
			} else {
			}
			inner.AddNext("span", attr("class", "toc_title")).SetText(p.Book.Title)
		} else {
			var notFirst bool
			for l := range strings.SplitSeq(te.title, "\n") {
				t := strings.TrimSpace(l)
				if len(t) > 0 {
					if notFirst {
						inner.AddNext("br").SetTail(t)
					} else {
						inner.SetText(t)
						notFirst = true
					}
				}
			}
		}
	}
	return nil
}

// generateCover creates proper cover page for the book.
func (p *Processor) generateCover() error {

	if len(p.Book.Cover) == 0 {
		return nil
	}

	p.env.Log.Debug("Generating cover page - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Generating cover page - done", zap.Duration("elapsed", time.Since(start)))
	}(time.Now())

	kindle := p.format == OMobi || p.format == OAzw3
	convert := p.env.Cfg.Doc.Cover.Convert
	w, h := p.env.Cfg.Doc.Cover.Width, p.env.Cfg.Doc.Cover.Height

	var cover *binImage
	for _, b := range p.Book.Images {
		if b.id == p.Book.Cover {
			cover = b
			break
		}
	}
	if cover == nil {
		p.env.Log.Warn("Unable to find specified cover image, disabling cover", zap.String("ref", p.Book.Cover))
		p.Book.Cover = ""
		return nil
	}

	if cover.img == nil {
		p.env.Log.Warn("unable to process specified cover image, disabling cover", zap.String("ref", p.Book.Cover))
		p.Book.Cover = ""
		return nil
	}

	// resize if needed
	switch p.coverResize {
	case CoverNone:
		if !convert && (!kindle || cover.img.Bounds().Dy() >= h) {
			break
		}
		fallthrough
	case CoverKeepAR:
		if img := imaging.Resize(cover.img, h*cover.img.Bounds().Dx()/cover.img.Bounds().Dy(), h, imaging.Lanczos); img != nil {
			cover.img = img
			cover.flags |= imageChanged
			if kindle || convert {
				cover.flags |= imageKindle
			}
		} else {
			p.env.Log.Warn("Unable to resize cover image, using as is")
		}
	case CoverStretch:
		if img := imaging.Resize(cover.img, w, h, imaging.Lanczos); img != nil {
			cover.img = img
			cover.flags |= imageChanged
			if kindle {
				cover.flags |= imageKindle
			}
		} else {
			p.env.Log.Warn("Unable to resize cover image, using as is")
		}
	}

	// stamp cover if requested
	if p.stampPlacement != StampNone {
		switch img, err := p.stampCover(cover.img); {
		case err != nil:
			p.env.Log.Warn("Unable to stamp cover image, using as is", zap.Error(err))
		case img == nil:
			// nothing to do
		default:
			cover.img = img
			cover.flags |= imageChanged
			if kindle {
				cover.flags |= imageKindle
			}
		}
	}

	if !kindle {
		// resizing will be done on device
		to, f := p.ctx().createXHTML("cover", attr("xmlns", `http://www.w3.org/1999/xhtml`))
		f.id = "cover-page"
		// Cover page goes first
		p.Book.Files = append(p.Book.Files, nil)
		copy(p.Book.Files[1:], p.Book.Files[0:])
		p.Book.Files[0] = f

		to.AddNext("svg",
			attr("version", "1.1"),
			attr("xmlns", "http://www.w3.org/2000/svg"),
			attr("xmlns:xlink", "http://www.w3.org/1999/xlink"),
			attr("width", "100%"),
			attr("height", "100%"),
			attr("viewBox", fmt.Sprintf("0 0 %d %d", w, h)),
			attr("preserveAspectRatio", "xMidYMid meet"),
		).AddNext("image",
			attr("width", fmt.Sprintf("%d", w)),
			attr("height", fmt.Sprintf("%d", h)),
			attr("xlink:href", path.Join(DirImages, cover.fname)),
		)
	}
	return nil
}

// simple stack data structure to help with NCX generation

type stackItem struct {
	level int
	elem  *etree.Element
}

type stackTOC struct {
	data []*stackItem
}

func (ts *stackTOC) depth() int {
	return len(ts.data)
}

func (ts *stackTOC) push(level int, value *etree.Element) {
	ts.data = append(ts.data, &stackItem{level, value})
}

func (ts *stackTOC) pop() (int, *etree.Element) {
	value := ts.data[len(ts.data)-1]
	ts.data[len(ts.data)-1] = nil
	ts.data = ts.data[:len(ts.data)-1]
	return value.level, value.elem
}

func (ts *stackTOC) peek(value *etree.Element) (int, *etree.Element) {
	if len(ts.data) > 0 {
		e := ts.data[len(ts.data)-1]
		return e.level, e.elem
	}
	return 0, value
}

// generateNCX creates Navigation Control file for XML applications.
func (p *Processor) generateNCX() error {

	p.env.Log.Debug("Generating NCX - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Generating NCX - done", zap.Duration("elapsed", time.Since(start)))
	}(time.Now())

	addNavPoint := func(to *etree.Element, index int, title, link string) *etree.Element {
		pt := to.AddNext("navPoint",
			attr("id", fmt.Sprintf("navpoint%d", index)),
			attr("playOrder", fmt.Sprintf("%d", index)),
		)
		pt.AddNext("navLabel").AddNext("text").SetText(title)
		pt.AddNext("content", attr("src", link))
		return pt
	}

	to, f := p.ctx().createNCX("toc", p.Book.ID.String())
	p.Book.Files = append(p.Book.Files, f)

	index := 1

	if p.tocPlacement == TOCBefore && len(p.Book.TOC) > 0 {
		addNavPoint(to, index, p.env.Cfg.Doc.TOC.Title, "toc.xhtml")
		index++
	}

	const (
		maxLevel       = int(math.MaxInt32)
		maxKindleLevel = 2
	)

	level := 2 // First (book title) on the same level as the rest, if you want everything be under it set level to -1
	barrier := maxLevel

	if p.tocType == TOCTypeFlat {
		level = maxLevel
		barrier = 1
	} else if p.tocType == TOCTypeKindle {
		level = maxKindleLevel
		barrier = 1
	}

	var (
		prev    *tocEntry
		history stackTOC
	)

	for _, e := range p.Book.TOC {
		switch {
		case prev == nil: // first time
			history.push(e.level.Int(), addNavPoint(to, index, AllLines(e.title), e.ref))
		case prev.level.Int() < e.level.Int(): // going in
			if e.level.Int() < level || history.depth() > barrier {
				history.pop()
			}
			_, inner := history.peek(to)
			history.push(e.level.Int(), addNavPoint(inner, index, AllLines(e.title), e.ref))
		case prev.level.Int() == e.level.Int(): // same level
			history.pop()
			_, inner := history.peek(to)
			history.push(e.level.Int(), addNavPoint(inner, index, AllLines(e.title), e.ref))
		case prev.level.Int() > e.level.Int(): // going out
			for l, elem := history.peek(nil); elem != nil && l >= e.level.Int(); l, elem = history.peek(nil) {
				history.pop()
			}
			_, inner := history.peek(to)
			history.push(e.level.Int(), addNavPoint(inner, index, AllLines(e.title), e.ref))
		default:
			panic("bad toc, should never happen")
		}
		prev = e
		index++
	}

	if p.tocPlacement == TOCAfter && len(p.Book.TOC) > 0 {
		addNavPoint(to, index, p.env.Cfg.Doc.TOC.Title, "toc.xhtml")
	}
	return nil
}

// process stylesheet and files it references.
func (p *Processor) prepareStylesheet() error {

	p.env.Log.Debug("Processing stylesheet - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Processing stylesheet - done", zap.Duration("elapsed", time.Since(start)))
	}(time.Now())

	d, err := p.getStylesheet()
	if err != nil {
		return err
	}

	processURL := func(index int, name string) string {

		if strings.Contains(name, "\\") {
			p.env.Log.Warn("Stylesheet has bad url with backslashes in the path. Trying to correct...", zap.String("url", name))
		}

		fname := name
		if !filepath.IsAbs(fname) {
			fname = filepath.Join(p.env.Cfg.Path, fname)
		}

		data, err := os.ReadFile(fname)
		if err != nil {
			p.env.Log.Warn("Stylesheet resource not found. Skipping...", zap.String("url", name))
			return name
		}

		d := &dataFile{data: data}
		switch {
		case isTTFFontFile(fname, data):
			d.id = fmt.Sprintf("font%d", index+1)
			d.fname = filepath.Base(fname)
			d.relpath = filepath.Join(DirContent, DirFonts)
			d.ct = "application/x-font-ttf"
		case isOTFFontFile(fname, data):
			d.id = fmt.Sprintf("font%d", index+1)
			d.fname = filepath.Base(fname)
			d.relpath = filepath.Join(DirContent, DirFonts)
			d.ct = "application/opentype"
		default:
			if strings.EqualFold(filepath.Ext(fname), ".ttf") || strings.EqualFold(filepath.Ext(fname), ".otf") {
				p.env.Log.Warn("Stylesheet font resource file format unrecognized (possibly wrong file extension). Skipping...", zap.String("url", name))
				return name
			}
			d.id = fmt.Sprintf("css_data%d", index+1)
			d.fname = "css_" + filepath.Base(fname)
			d.relpath = filepath.Join(DirContent, DirImages)
			d.ct = mime.TypeByExtension(filepath.Ext(fname))
		}

		p.Book.Data = append(p.Book.Data, d)
		return path.Join(DirFonts, d.fname)
	}

	// Get all references from stylesheet
	var (
		result    string
		lastIndex = 0
		pattern   = regexp.MustCompile(`url\(\s*"([^\s\(\)[:cntrl:]]+)"|'([^\s\(\)[:cntrl:]]+)'\s*\)`)
	)
	allIndexes := pattern.FindAllSubmatchIndex(d.data, -1)
	for i, loc := range allIndexes {
		var b, e int
		switch {
		case loc[2] > 0 && loc[3] > 0:
			// first group
			b, e = loc[2], loc[3]
		case loc[4] > 0 && loc[5] > 0:
			// second group
			b, e = loc[4], loc[5]
		default:
			continue
		}
		result += string(d.data[lastIndex:b]) + processURL(i, string(d.data[b:e]))
		lastIndex = e
	}
	result += string(d.data[lastIndex:])

	if len(result) > 0 {
		d.data = []byte(result)
	}
	return nil
}

// generatePagemap creates epub page map.
func (p *Processor) generatePagemap() error {

	if p.noPages {
		return nil
	}

	p.env.Log.Debug("Generating page map - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Generating page map - done", zap.Duration("elapsed", time.Since(start)))
	}(time.Now())

	to, f := p.ctx().createPM("page-map")
	p.Book.Files = append(p.Book.Files, f)

	page := 1
	for _, f := range p.Book.Files {
		if f.transient&dataNotForSpline != 0 {
			continue
		}

		to.AddNext("page", attr("name", fmt.Sprintf("%d", page)), attr("href", f.fname))
		page++

		additionalPages, ok := p.Book.Pages[f.fname]
		if !ok {
			continue
		}

		for i := 0; i < additionalPages; i++ {
			to.AddNext("page", attr("name", fmt.Sprintf("%d", page)), attr("href", fmt.Sprintf("%s#page_%d", f.fname, i)))
			page++
		}
	}
	return nil
}

// generateOPF creates epub Open Package format file.
func (p *Processor) generateOPF() error {

	p.env.Log.Debug("Generating OPF - start")
	defer func(start time.Time) {
		p.env.Log.Debug("Generating OPF - done", zap.Duration("elapsed", time.Since(start)))
	}(time.Now())

	to, f := p.ctx().createOPF("content")
	p.Book.Files = append(p.Book.Files, f)

	kindle := p.format == OMobi || p.format == OAzw3

	// Metadata generation

	meta := to.AddNext("metadata",
		attr("xmlns:dc", `http://purl.org/dc/elements/1.1/`),
		attr("xmlns:opf", `http://www.idpf.org/2007/opf`),
	)

	var title string
	if len(p.env.Cfg.Doc.TitleFormat) > 0 {
		if p.version == 1 {
			title = ReplaceKeywords(p.env.Cfg.Doc.TitleFormat, CreateTitleKeywordsMap(p.Book, p.env.Cfg.Doc.SeqNumPos, p.env.Cfg.Doc.SeqFirstWordLen, p.src))
		} else {
			var err error
			title, err = p.expandTemplate("opf-title", p.env.Cfg.Doc.TitleFormat, -1)
			if err != nil {
				return fmt.Errorf("unable to prepare title for opf-title using '%s': %w", p.env.Cfg.Doc.TitleFormat, err)
			}
		}
	}
	if len(title) == 0 {
		title = p.Book.Title
	}
	if p.env.Cfg.Doc.TransliterateMeta {
		title = slug.Make(title)
	}
	meta.AddNext("dc:title").SetText(title)
	meta.AddNext("dc:language").SetText(p.Book.Lang.String())
	meta.AddNext("dc:identifier", attr("id", "BookId"), attr("opf:scheme", "uuid")).SetText(fmt.Sprintf("urn:uuid:%s", p.Book.ID))

	for i, an := range p.Book.Authors {
		var a string
		if p.version == 1 {
			a = ReplaceKeywords(p.env.Cfg.Doc.AuthorFormatMeta, CreateAuthorKeywordsMap(an))
		} else {
			var err error
			a, err = p.expandTemplate("opf-author", p.env.Cfg.Doc.AuthorFormat, i)
			if err != nil {
				return fmt.Errorf("unable to prepare author for opr-author using '%s': %w", p.env.Cfg.Doc.AuthorFormat, err)
			}
		}
		if p.env.Cfg.Doc.TransliterateMeta {
			a = slug.Make(a)
		}
		meta.AddNext("dc:creator", attr("opf:role", "aut")).SetText(a)
	}

	meta.AddNext("dc:publisher")

	for _, g := range p.Book.Genres {
		meta.AddNext("dc:subject").SetText(g)
	}

	if len(p.Book.Annotation) > 0 {
		meta.AddNext("dc:description").SetText(p.Book.Annotation)
	}

	// Amazon and Apple like this, but its epub3
	if len(p.Book.Cover) > 0 {
		meta.AddNext("meta", attr("name", "cover"), attr("content", "book-cover-image"))
	}
	// Do not let series metadata to disappear, use calibre meta tags
	if len(p.Book.SeqName) > 0 {
		meta.AddNext("meta", attr("name", "calibre:series"), attr("content", p.Book.SeqName))
		if p.Book.SeqNum > 0 {
			meta.AddNext("meta", attr("name", "calibre:series_index"), attr("content", strconv.Itoa(p.Book.SeqNum)))
		}
	}

	// Manifest generation

	man := to.AddNext("manifest")

	for _, f := range p.Book.Files {
		if f.transient&dataNotForManifest != 0 {
			continue
		}
		man.AddSame("item", attr("id", f.id), attr("media-type", f.ct), attr("href", f.fname))
	}

	for i, f := range p.Book.Images {
		if f.id == p.Book.Cover {
			man.AddSame("item",
				attr("id", "book-cover-image"),
				attr("media-type", f.ct),
				attr("href", path.Join(DirImages, f.fname)),
				attr("properties", "cover-image"))
		} else {
			man.AddSame("item",
				attr("id", fmt.Sprintf("image%d", i+1)),
				attr("media-type", f.ct),
				attr("href", path.Join(DirImages, f.fname)))
		}
	}

	for i, f := range p.Book.Vignettes {
		man.AddSame("item",
			attr("id", fmt.Sprintf("vignette%d", i+1)),
			attr("media-type", f.ct),
			attr("href", path.Join(DirVignettes, f.fname)))
	}

	for _, f := range p.Book.Data {
		if f.transient&dataNotForManifest != 0 {
			continue
		}
		man.AddSame("item",
			attr("id", f.id),
			attr("media-type", f.ct),
			attr("href", path.Join(strings.TrimPrefix(strings.TrimPrefix(f.relpath, DirContent), string(filepath.Separator)), f.fname)))
	}

	// Spine generation

	var spine *etree.Element
	if p.noPages {
		spine = to.AddNext("spine", attr("toc", "ncx"))
	} else {
		spine = to.AddNext("spine", attr("toc", "ncx"), attr("page-map", "page-map"))
	}

	for _, f := range p.Book.Files {
		id := f.id
		if f.transient&dataNotForSpline != 0 {
			continue
		}
		if len(id) == 0 {
			id = strings.TrimSuffix(filepath.Base(f.fname), filepath.Ext(f.fname))
		}
		attrs := append(make([]*etree.Attr, 0, 2), attr("idref", id))
		if id == "cover-page" {
			attrs = append(attrs, attr("linear", "no"))
		}
		spine.AddSame("itemref", attrs...)
	}

	// Guide generation

	guide := to.AddNext("guide")

	if len(p.Book.Cover) > 0 && !kindle {
		guide.AddSame("reference", attr("type", "cover-page"), attr("title", "Starts here"), attr("href", "cover.xhtml"))
	}

	started := false
	if len(p.Book.Cover) > 0 && p.env.Cfg.Doc.OpenFromCover && !kindle {
		guide.AddSame("reference", attr("type", "text"), attr("title", "Starts here"), attr("href", "cover.xhtml"))
		started = true
	}
	if !started && p.env.Cfg.Doc.OpenFromCover && kindle {
		// find annotation file
		for _, f := range p.Book.Files {
			if strings.HasPrefix(f.fname, "annotation") {
				guide.AddSame("reference", attr("type", "text"), attr("title", "Starts here"), attr("href", f.fname))
				started = true
				break
			}
		}
	}
	if !started {
		// find first content file
		for _, f := range p.Book.Files {
			if strings.HasPrefix(f.fname, "index") {
				guide.AddSame("reference", attr("type", "text"), attr("title", "Starts here"), attr("href", f.fname))
				break
			}
		}
	}

	if p.tocPlacement != TOCNone {
		guide.AddSame("reference", attr("type", "toc"), attr("title", "Table of Contents"), attr("href", "toc.xhtml"))
	}

	return nil
}

// generateMeta creates files necessary for Open Container Format.
func (p *Processor) generateMeta() error {

	_, f := p.ctx().createOCF("container")
	p.Book.Meta = append(p.Book.Meta, f,
		&dataFile{
			id:        "mimetype",
			fname:     "mimetype",
			transient: dataNotForSpline | dataNotForManifest,
			ct:        "text/plain",
			data:      []byte(`application/epub+zip`),
		})

	return nil
}

// KepubifyXHTML inserts Kobo specific formatting into results.
func (p *Processor) KepubifyXHTML() error {

	if p.format != OKepub {
		return nil
	}

	for _, f := range p.Book.Files {
		if f.ct == "application/xhtml+xml" && filepath.Ext(f.fname) == ".xhtml" && f.doc != nil {
			if body := f.doc.FindElement("./html/body"); body != nil {
				to := etree.NewElement("div")
				to.CreateAttr("id", "book-columns")
				inner := to.AddNext("div", attr("id", "book-inner"))
				children := body.ChildElements()
				for i := 0; i < len(children); i++ {
					inner.AddChild(body.RemoveChild(children[i]))
				}
				body.AddChild(to)
			}
		}
	}
	return nil
}
