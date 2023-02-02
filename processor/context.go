package processor

import (
	"fmt"
	"path/filepath"

	"fb2converter/etree"
)

// context used during fb2 transformation.
type context struct {
	findex            int
	fname             string
	pageLength        int
	out               *etree.Document
	bodyName          string
	firstBodyTitle    bool        // first title in a body needs special processing
	firstChapterLine  bool        // we need to know when to process drop caps
	specialParagraph  bool        // special paragraph processing, no drop caps
	sectionWithTitle  stackedBool // indicates that current section has title
	sectionTextLength stackedInt  // has current section text length - paragraphs only
	paragraph         int         // used for Kobo spans
	sentence          int         // used for Kobo spans
	inParagraph       bool
	inHeader          bool
	inSubHeader       bool
	header            htmlHeader
	tocIndex          int
	currentNotes      []*note // for inline and block notes
	debug             bool
}

// newContext creates new empty parsing context.
func newContext() *context {
	c := &context{
		out:      etree.NewDocument(),
		tocIndex: 1,
	}
	c.out.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}
	c.sectionWithTitle.link()
	c.sectionTextLength.link()
	return c
}

func (ctx *context) createXHTML(name string, attrs ...*etree.Attr) (*etree.Element, *dataFile) {

	var fname string
	if len(name) == 0 {
		// main body
		if len(ctx.bodyName) == 0 {
			fname = fmt.Sprintf("index%d", ctx.findex+1)
			ctx.findex++
		} else {
			fname = GenSafeName(ctx.bodyName)
		}
	} else {
		fname = name
	}
	ctx.fname = fname + ".xhtml"
	ctx.pageLength = 0
	ctx.paragraph = 0

	// set up XML
	ctx.out = etree.NewDocument()
	ctx.out.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}
	ctx.out.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	f := &dataFile{
		id:      fname,
		fname:   ctx.fname,
		relpath: DirContent,
		ct:      "application/xhtml+xml",
		doc:     ctx.out,
	}

	html := ctx.out.Element.AddNext("html", attrs...)

	html.AddNext("head").
		AddSame("meta", attr("http-equiv", "Content-Type"), attr("content", `text/html; charset=utf-8`)).
		AddSame("link", attr("rel", "stylesheet"), attr("type", "text/css"), attr("href", "stylesheet.css")).
		AddNext("title").SetText("fb2converter")

	return html.AddNext("body"), f
}

func (ctx *context) createNCX(name, id string) (*etree.Element, *dataFile) {

	ctx.fname = name + ".ncx"
	ctx.pageLength = 0
	ctx.paragraph = 0

	// set up XML
	ctx.out = etree.NewDocument()
	ctx.out.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}
	ctx.out.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	f := &dataFile{
		id:        "ncx",
		fname:     ctx.fname,
		relpath:   DirContent,
		transient: dataNotForSpline,
		ct:        "application/x-dtbncx+xml",
		doc:       ctx.out,
	}

	ncx := ctx.out.Element.AddNext("ncx",
		attr("xmlns", `http://www.daisy.org/z3986/2005/ncx/`),
		attr("version", "2005-1"),
		attr("xml:lang", "en-US"),
	)

	ncx.AddNext("head").AddNext("meta", attr("name", "dtb:uid"), attr("content", fmt.Sprintf("urn:uuid:%s", id)))
	ncx.AddNext("docTitle").AddNext("text").SetText("fb2converter")

	return ncx.AddNext("navMap"), f
}

func (ctx *context) createPM(name string) (*etree.Element, *dataFile) {

	ctx.fname = name + ".xml"
	ctx.pageLength = 0
	ctx.paragraph = 0

	// set up XML
	ctx.out = etree.NewDocument()
	ctx.out.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}
	ctx.out.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	f := &dataFile{
		id:        name,
		fname:     ctx.fname,
		relpath:   DirContent,
		transient: dataNotForSpline,
		ct:        "application/oebps-page-map+xml",
		doc:       ctx.out,
	}
	pm := ctx.out.Element.AddNext("page-map", attr("xmlns", `http://www.idpf.org/2007/opf`))

	return pm, f
}

func (ctx *context) createOPF(name string) (*etree.Element, *dataFile) {

	ctx.fname = name + ".opf"
	ctx.pageLength = 0
	ctx.paragraph = 0

	// set up XML
	ctx.out = etree.NewDocument()
	ctx.out.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}
	ctx.out.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	f := &dataFile{
		id:        "content",
		fname:     ctx.fname,
		relpath:   DirContent,
		ct:        "application/xhtml+xml",
		transient: dataNotForSpline | dataNotForManifest,
		doc:       ctx.out,
	}

	pkg := ctx.out.Element.AddNext("package",
		attr("version", "2.0"),
		attr("xmlns", `http://www.idpf.org/2007/opf`),
		attr("unique-identifier", "BookId"),
	)

	return pkg, f
}

func (ctx *context) createOCF(name string) (*etree.Element, *dataFile) {

	ctx.fname = name + ".xml"
	ctx.pageLength = 0
	ctx.paragraph = 0

	// set up XML
	ctx.out = etree.NewDocument()
	ctx.out.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}
	ctx.out.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	f := &dataFile{
		id:        name,
		fname:     ctx.fname,
		relpath:   DirMata,
		transient: dataNotForSpline | dataNotForManifest,
		ct:        "text/xml",
		doc:       ctx.out,
	}

	ocf := ctx.out.Element.AddNext("container",
		attr("version", "1.0"),
		attr("xmlns", `urn:oasis:names:tc:opendocument:xmlns:container`)).
		AddNext("rootfiles").
		AddNext("rootfile",
			attr("full-path", filepath.ToSlash(filepath.Join(DirContent, "content.opf"))),
			attr("media-type", "application/oebps-package+xml"))

	return ocf, f
}

// Stacked variables allow context to keep values localized within come arbitrary boundaries, rather than using stack form function calls. This is
// useful to keep contextual state across (recursive) function calls if necessary.

type stackedBool struct {
	ptr *bool
}

//lint:ignore U1000 keep val()
func (pb *stackedBool) val() bool {
	return *pb.ptr
}

func (pb *stackedBool) set() {
	*pb.ptr = true
}

func (pb *stackedBool) link() func() bool {

	var (
		value bool
		ptr   *bool
	)
	ptr, pb.ptr = pb.ptr, &value

	return func() bool {
		v := *pb.ptr
		pb.ptr = ptr
		return v
	}
}

type stackedInt struct {
	ptr *int
}

//lint:ignore U1000 keep val()
func (pi *stackedInt) val() int {
	return *pi.ptr
}

//lint:ignore U1000 keep val()
func (pi *stackedInt) set(val int) {
	*pi.ptr = val
}

func (pi *stackedInt) add(val int) {
	*pi.ptr += val
}

func (pi *stackedInt) link() func() int {

	var (
		value int
		ptr   *int
	)
	ptr, pi.ptr = pi.ptr, &value

	return func() int {
		v := *pi.ptr
		pi.ptr = ptr
		return v
	}
}
