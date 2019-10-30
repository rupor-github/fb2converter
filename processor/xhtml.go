package processor

import (
	"fmt"
	"html"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/asaskevich/govalidator"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/rupor-github/fb2converter/config"
	"github.com/rupor-github/fb2converter/etree"
)

// processBody parses fb2 document body and produces formatted output.
func (p *Processor) processBody(index int, from *etree.Element) (err error) {

	// setup processing context
	if index != 0 {
		// always ignore first body name - it is main body
		p.ctx().bodyName = getAttrValue(from, "name")
	}

	p.ctx().header = 0
	p.ctx().firstBodyTitle = true

	start := time.Now()
	p.env.Log.Debug("Parsing body - start", zap.String("name", p.ctx().bodyName))
	defer func(start time.Time) {
		p.env.Log.Debug("Parsing body - done",
			zap.Duration("elapsed", time.Since(start)),
			zap.String("name", p.ctx().bodyName),
		)
	}(start)

	if p.notesMode == NDefault || !IsOneOf(p.ctx().bodyName, p.env.Cfg.Doc.Notes.BodyNames) {
		// initialize first XHTML buffer
		to, f := p.ctx().createXHTML("")
		p.Book.Files = append(p.Book.Files, f)
		p.Book.Pages[f.fname] = 0
		return p.transfer(from, to)
	}

	if p.notesMode != NFloat && p.notesMode != NFloatOld {
		// NOTE: for block and inline notes we do not need to save XHTML, have nothing to put there
		return nil
	}

	// initialize XHTML buffer for notes
	to, f := p.ctx().createXHTML("")
	p.Book.Files = append(p.Book.Files, f)

	// To satisfy Amazon's requirements for floating notes we have to create notes body on the fly here, removing most if not
	// all of existing formatting. At this point we already scanned available notes in ProcessNotes()...
	for i, nl := range p.Book.NotesOrder {

		// title section
		if i == 0 {
			tocRefID := fmt.Sprintf("tocref%d", p.ctx().tocIndex)
			inner := to.AddNext("div", attr("class", "titleblock"), attr("id", tocRefID))

			vignette := p.getVignetteFile("h0", config.VigBeforeTitle)
			if len(vignette) > 0 {
				inner.AddNext("div", attr("class", "vignette_title_before")).
					AddNext("img", attr("src", path.Join("vignettes", filepath.Base(vignette))))
			}

			var tocTitle string
			if t, ok := p.Book.NoteBodyTitles[p.ctx().bodyName]; ok {
				tocTitle = t.title
				inner.AddChild(t.parsed.Copy())
			} else {
				tocTitle = strings.Title(p.ctx().bodyName)
				inner.AddNext("div", attr("class", "h0")).AddNext("p", attr("class", "title")).SetText(tocTitle)
			}

			vignette = p.getVignetteFile("h0", config.VigAfterTitle)
			if len(vignette) > 0 {
				inner.AddNext("div", attr("class", "vignette_title_after")).
					AddNext("img", attr("src", path.Join("vignettes", filepath.Base(vignette))))
			}

			p.Book.TOC = append(p.Book.TOC, &tocEntry{
				ref:      p.ctx().fname + "#" + tocRefID,
				title:    tocTitle,
				level:    p.ctx().header,
				bodyName: p.ctx().bodyName,
			})
			p.ctx().tocIndex++
		}

		// note body
		if nl.bodyName == p.ctx().bodyName {
			note := p.Book.Notes[nl.id]
			backID := "back_" + nl.id
			p.Book.LinksLocations[nl.id] = p.ctx().fname
			backRef, exists := p.Book.LinksLocations[backID]
			if !exists {
				backRef = "nowhere"
			}
			t := "***."
			if len(note.title) > 0 {
				t = note.title
				// Sometimes authors put "." inside the note
				if !strings.HasSuffix(t, ".") {
					t += "."
				}
			}

			if p.notesMode == NFloatOld {
				// old bi-directional mode
				to.AddNext("p", attr("class", "floatnote"), attr("id", nl.id)).
					AddNext("a", attr("href", backRef+"#"+backID)).SetText(t).SetTail(strNBSP + note.body)
			} else {
				// new "preffered" HTML5 method with "aside"
				to.AddNext("aside", attr("id", nl.id), attr("epub:type", "footnote")).
					AddNext("p", attr("class", "floatnote")).
					AddNext("a", attr("href", backRef+"#"+backID)).SetText(t).SetTail(strNBSP + note.body)
			}
		}
	}
	return nil
}

func (p *Processor) doTextTransformations(text string, breakable, tail bool) string {

	if p.ctx().inParagraph && breakable {
		// normalize direct speech if requested
		if !tail && p.speechTransform != nil {
			from, to := p.speechTransform.From, p.speechTransform.To
			cutIndex := 0
			for i, sym := range text {
				if i == 0 {
					if !strings.ContainsRune(from, sym) {
						break
					}
					cutIndex += utf8.RuneLen(sym)
				} else {
					if unicode.IsSpace(sym) {
						cutIndex += utf8.RuneLen(sym)
					} else {
						text = to + text[cutIndex:]
						break
					}
				}
			}
		}

		// unify dashes if requested
		if p.dashTransform != nil {
			var (
				b     strings.Builder
				runes = []rune(text)
			)
			for i := 0; i < len(runes); i++ {
				if i > 0 && unicode.IsSpace(runes[i-1]) &&
					i < len(runes)-1 && unicode.IsSpace(runes[i+1]) &&
					strings.ContainsRune(p.dashTransform.From, runes[i]) {

					b.WriteString(p.dashTransform.To)
					continue
				}
				b.WriteRune(runes[i])
			}
			text = b.String()
		}
	}
	return text
}

// formatText inserts page markers (for page map), kobo spans (if necessary) and hyphenates words if requested.
func (p *Processor) formatText(in string, breakable, tail bool, to *etree.Element) {

	in = p.doTextTransformations(in, breakable, tail)

	var (
		textOut             string
		textOutLen          int  // before hyphenation
		dropcapFound        bool // if true - do not look for dropcap
		buf                 strings.Builder
		page, insertMarkers = p.Book.Pages[p.ctx().fname]
		kobo                = p.format == OKepub
	)

	bufWriteString := func(text string, kobo bool) {
		if kobo {
			p.ctx().sentence++
			buf.WriteString(`<span class="koboSpan" id=` + fmt.Sprintf("\"kobo.%d.%d\"", p.ctx().paragraph, p.ctx().sentence) + `>`)
		}
		buf.WriteString(html.EscapeString(text))
		if kobo {
			buf.WriteString(`</span>`)
		}
	}

	buf.WriteString(`<root>`)

	for k, sentence := range splitSentences(p.Book.tokenizer, in) {
		for i, word := range splitWords(p.Book.tokenizer, sentence, p.env.Cfg.Doc.NoNBSP) {

			wl := utf8.RuneCountInString(word)

			dropIndex := 0
			if k == 0 && i == 0 && wl > 0 && !dropcapFound && // worth looking and we still do not have it
				p.ctx().inParagraph && breakable && p.env.Cfg.Doc.DropCaps.Create && !tail &&
				p.ctx().firstChapterLine && !p.ctx().inHeader && !p.ctx().inSubHeader && len(p.ctx().bodyName) == 0 && !p.ctx().specialParagraph {

				for j, sym := range word {
					if !strings.ContainsRune(p.env.Cfg.Doc.DropCaps.IgnoreSymbols, sym) {
						// Do not dropcap spaces unless they are set as ignored
						if !unicode.IsSpace(sym) {
							dropIndex = j + utf8.RuneLen(sym)
							dropcapFound = true
						}
						break
					}
				}

				if dropIndex > 0 {
					buf.WriteString(`<span class="dropcaps">`)
					bufWriteString(word[0:dropIndex], false)
					buf.WriteString(`</span>`)
					word = word[dropIndex:]
				}
			}

			if p.Book.hyph != nil && !p.ctx().inHeader && !p.ctx().inSubHeader && wl > 2 && dropIndex == 0 {
				word = p.Book.hyph.hyphenate(word)
			}

			textOutLen += wl
			if i == 0 {
				textOut = word
			} else {
				textOut = strings.Join([]string{textOut, word}, " ")
				textOutLen++ // count extra space
			}

			if p.ctx().inParagraph && !p.ctx().inHeader && !p.ctx().inSubHeader && len(p.ctx().bodyName) == 0 {
				// to properly set chapter_end vignette we need to know if chapter has some text in it
				p.ctx().sectionTextLength.add(textOutLen)
			}

			if insertMarkers && p.ctx().inParagraph && breakable && p.ctx().pageLength+textOutLen >= p.env.Cfg.Doc.CharsPerPage {
				if len(textOut) > 0 {
					bufWriteString(textOut, kobo)
				}
				buf.WriteString(`<a class="pagemarker" id=` + fmt.Sprintf("\"page_%d\"/>", page))
				p.ctx().pageLength, textOutLen, textOut = 0, 0, ""
				page++
			}
		}
		if len(textOut) > 0 {
			bufWriteString(textOut, kobo)
			p.ctx().pageLength, textOutLen, textOut = p.ctx().pageLength+textOutLen, 0, ""
		}
	}

	buf.WriteString(`</root>`)

	if insertMarkers {
		p.Book.Pages[p.ctx().fname] = page
	}

	if !tail &&
		p.ctx().firstChapterLine && !p.ctx().inHeader && !p.ctx().inSubHeader && len(p.ctx().bodyName) == 0 && !p.ctx().specialParagraph {
		// we are looking for drop cups on a first line of chapter only
		p.ctx().firstChapterLine = false
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromString(buf.String()); err != nil {
		p.env.Log.Error("Unable to format text", zap.String("text", buf.String()), zap.Error(err))
	}

	if tail {
		text := doc.Root().Text()
		if len(text) > 0 {
			to.SetTail(text)
		}
		pel := to.Parent()
		for _, e := range doc.Root().ChildElements() {
			pel.AddChild(e)
		}
	} else {
		if dropcapFound {
			if attr := to.SelectAttr("class"); attr != nil {
				attr.Value = "dropcaps"
			} else {
				to.CreateAttr("class", "dropcaps")
			}
		}
		text := doc.Root().Text()
		if len(text) > 0 {
			to.SetText(text)
		}
		for _, e := range doc.Root().ChildElements() {
			to.AddChild(e)
		}
	}
}

// transfer converts source xml element to resulting xhtml fragment, possibly with multiple nodes and formatting.
// NOTE: decorations (if any) are (order important): name of new html tag, its css class, href attribute.
func (p *Processor) transfer(from, to *etree.Element, decorations ...string) error {

	// See if decorations are requested
	var tag, css, href string
	for i, p := range decorations {
		switch {
		case i == 0:
			tag = p
		case i == 1:
			css = p
		case i == 2:
			href = p
		}
	}

	text := from.Text()
	tail := from.Tail()

	// links are notes - probably
	if tag == "a" && len(href) > 0 {
		var noteID string
		// Some people does not know how to format url properly
		href = strings.Replace(href, "\\", "/", -1)
		if u, err := url.Parse(href); err != nil {
			p.env.Log.Warn("unable to parse note href", zap.String("href", href), zap.Error(err))
		} else {
			noteID = u.Fragment
			switch p.notesMode {
			case NDefault:
				if _, ok := p.Book.Notes[noteID]; !ok {
					css = "linkanchor"
				}
			case NFloat:
				fallthrough
			case NFloatOld:
				if _, ok := p.Book.Notes[noteID]; !ok {
					css = "linkanchor"
				} else {
					// NOTE: modifying attribute on SOURCE node!
					from.CreateAttr("id", "back_"+noteID)
				}
			case NInline:
				fallthrough
			case NBlock:
				if n, ok := p.Book.Notes[noteID]; ok {
					p.ctx().currentNotes = append(p.ctx().currentNotes, n)
					tag = "span"
					css = fmt.Sprintf("%sanchor", p.notesMode.String())
					href = ""
				}
			default:
				return errors.New("unknown notes mode - this should never happen")
			}
		}
	}

	// generate requested node at destination
	inner := to
	if len(tag) > 0 {
		var newid string
		if id := getAttrValue(from, "id"); len(id) > 0 {
			var changed bool
			newid, changed = SanitizeName(id)
			if changed {
				p.env.Log.Warn("Tag id was sanitized. This may create problems with links (TOC, notes) - it is better to fix original file", zap.String(tag, id))
			}
			p.Book.LinksLocations[newid] = p.ctx().fname
		}
		// NOTE: There could be sections inside sections to no end, so we do not want to repeat this as it will break TOC on "strangly" formatted texts,
		// we will just mark main section beginning with "section" css in case somebody wants to do some formatting there
		if css == "section" {
			attrs := make([]*etree.Attr, 2)
			attrs[0] = attr("class", css)
			attrs[1] = attr("href", href)
			if len(newid) != 0 {
				attrs = append(attrs, attr("id", newid))
			}
			to.AddNext(tag, attrs...)
		} else {
			attrs := make([]*etree.Attr, 3)
			attrs[0] = attr("id", newid)
			attrs[1] = attr("class", css)
			attrs[2] = attr("href", href)
			if p.notesMode == NFloat && tag == "a" {
				attrs = append(attrs, attr("epub:type", "noteref"))
			}
			inner = to.AddNext(tag, attrs...)
		}
		if tag == "p" {
			p.ctx().inParagraph = true
			defer func() { p.ctx().inParagraph = false }()
			p.ctx().paragraph++
			p.ctx().sentence = 0
		}
	}

	// add node text
	if len(text) > 0 {
		p.formatText(text, from.Tag == "p", false, inner)
	}

	// transfer children
	var err error
	for _, child := range from.ChildElements() {
		if proc, ok := supportedTransfers[child.Tag]; ok {
			err = proc(p, child, inner)
			if err == nil && from.Tag == "section" {
				// NOTE: during inner section transfer we may open new xhtml file starting new chapter, so we want to sync up current node...
				if body := p.ctx().out.FindElement("./html/body"); body != nil {
					to, inner = body, body
				}
			}
		} else {
			// unexpected tag to transfer
			if from.Tag == "body" || from.Tag == "section" {
				p.env.Log.Debug("Unexpected tag, ignoring completely", zap.String("tag", from.Tag), zap.String("xml", getXMLFragmentFromElement(child)))
				continue
			}
			p.env.Log.Debug("Unexpected tag, transferring", zap.String("tag", from.Tag), zap.String("xml", getXMLFragmentFromElement(child)))
			// NOTE: all "unknown" attributes will be lost during this transfer
			err = p.transfer(child, inner, child.Tag)
		}
		if err != nil {
			return err
		}
	}

	// add bodies of inline and block notes
	// NOTE: presently ignore note bodies when calculating page length...
	currentNotes := p.ctx().currentNotes
	if len(p.ctx().currentNotes) > 0 {
		// insert inline and block notes
		if p.notesMode == NInline && tag == "span" {
			inner = to.AddNext("span", attr("class", "inlinenote")).SetText(currentNotes[0].body)
			p.ctx().currentNotes = []*note{}
		} else if p.notesMode == NBlock && tag == "p" {
			inner := to.AddNext("div", attr("class", "blocknote"))
			for _, n := range currentNotes {
				t := n.title
				if i, err := strconv.Atoi(t); err == nil {
					t = fmt.Sprintf("%d) ", i)
				}
				inner.AddNext("p").AddNext("span", attr("class", "notenum")).SetText(t).SetTail(n.body)
			}
			p.ctx().currentNotes = []*note{}
		}
	}

	// and do not forget node tail
	if len(tail) > 0 {
		p.formatText(tail, from.Tag == "p", true, inner)
	}
	return nil
}

var supportedTransfers map[string]func(p *Processor, from, to *etree.Element) error

func init() {

	// all tags mentioned in "http://www.gribuser.ru/xml/fictionbook/2.2/xsd/FictionBook2.2.xsd" and then some
	supportedTransfers = map[string]func(p *Processor, from, to *etree.Element) error{

		"title":   transferTitle,
		"image":   transferImage,
		"section": transferSection,
		"span":    transferSpan,
		"subtitle": func(p *Processor, from, to *etree.Element) error {
			p.ctx().inSubHeader = true
			defer func() { p.ctx().inSubHeader = false }()
			return p.transfer(from, to, "p", "subtitle")
		},
		"epigraph": func(p *Processor, from, to *etree.Element) error {
			p.ctx().specialParagraph = true
			defer func() { p.ctx().specialParagraph = false }()
			return p.transfer(from, to, "div", "epigraph")
		},
		"annotation": func(p *Processor, from, to *etree.Element) error {
			p.ctx().specialParagraph = true
			defer func() { p.ctx().specialParagraph = false }()
			return p.transfer(from, to, "div", "annotation")
		},
		"b": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "span", "strong")
		},
		"strong": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "span", "strong")
		},
		"i": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "span", "emphasis")
		},
		"emphasis": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "span", "emphasis")
		},
		"strikethrough": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "span", "strike")
		},
		"style": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "span")
		},
		"a": transferAnchor,
		"p": transferParagraph,
		"poem": func(p *Processor, from, to *etree.Element) error {
			p.ctx().specialParagraph = true
			defer func() { p.ctx().specialParagraph = false }()
			return p.transfer(from, to, "div", "poem")
		},
		"stanza": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "div", "stanza")
		},
		"v": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "p")
		},
		"cite": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "div", "cite")
		},
		"empty-line": func(_ *Processor, _, to *etree.Element) error { //nolint:unparam
			to.AddNext("div", attr("class", "emptyline"))
			return nil
		},
		"text-author": func(p *Processor, from, to *etree.Element) error {
			p.ctx().specialParagraph = true
			defer func() { p.ctx().specialParagraph = false }()
			return p.transfer(from, to, "div", "text-author")
		},
		"code": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "code")
		},
		"date": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "time")
		},
		"sup": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "sup")
		},
		"sub": func(p *Processor, from, to *etree.Element) error {
			return p.transfer(from, to, "sub")
		},
		"table": transferTable,
		"tr":    transferTableElement,
		"td":    transferTableElement,
		"th":    transferTableElement,
	}
}

func transferParagraph(p *Processor, from, to *etree.Element) error {

	// Split content if requested
	if pages, ok := p.Book.Pages[p.ctx().fname]; ok && pages >= p.env.Cfg.Doc.PagesPerFile &&
		!p.ctx().inHeader && !p.ctx().inSubHeader && len(p.ctx().bodyName) == 0 && !p.ctx().specialParagraph {

		// open next XHTML
		var f *dataFile
		to, f = p.ctx().createXHTML("")
		// store it for future flushing
		p.Book.Files = append(p.Book.Files, f)
		p.Book.Pages[f.fname] = 0
	}

	var css string
	if p.ctx().inHeader {
		css = "title"
	}
	return p.transfer(from, to, "p", css)
}

func transferAnchor(p *Processor, from, to *etree.Element) error {
	href := getAttrValue(from, "href")
	if len(href) == 0 {
		txt := strings.TrimSpace(from.Text())
		if len(txt) > 0 && len(from.ChildElements()) == 0 {
			// some idiots think that anchors are for text formatting - see if we could save it
			href = strings.Trim(filepath.ToSlash(txt), ".,")
			if govalidator.IsURL(href) || govalidator.IsEmail(href) {
				return p.transfer(from, to, "a", "anchor", href)
			}
		}
		if len(txt) > 0 || len(from.ChildElements()) > 0 {
			p.env.Log.Warn("Unable to find href attribute in anchor", zap.String("xml", getXMLFragmentFromElement(from)))
			return p.transfer(from, to, "a", "empty-href")
		}
		p.env.Log.Warn("Unable to find href attribute in anchor, ignoring", zap.String("xml", getXMLFragmentFromElement(from)))
		return nil
	}
	// sometimes people are doing strange things with URLs
	return p.transfer(from, to, "a", "anchor", filepath.ToSlash(href))
}

func transferTitle(p *Processor, from, to *etree.Element) error {

	defer func() {
		p.ctx().inHeader = false
		p.ctx().firstBodyTitle = false
		p.ctx().tocIndex++
	}()

	tocRefID := fmt.Sprintf("tocref%d", p.ctx().tocIndex)
	tocTitle := SanitizeTitle(getTextFragment(from))

	// notes bodies have many titles, for main body only first title deserves special processing
	if len(p.ctx().bodyName) == 0 || p.ctx().firstBodyTitle {

		// to properly set chapter_end vignette we need to know if chapter has title
		p.ctx().sectionWithTitle.set()

		p.ctx().inHeader = true
		p.ctx().firstChapterLine = true

		cls := "titleblock"
		if p.ctx().header.Int() >= p.env.Cfg.Doc.ChapterLevel {
			cls = "titleblock_nobreak"
		}
		div := to.AddNext("div", attr("id", tocRefID), attr("class", cls))

		h := p.ctx().header.String("h")
		vignette := p.getVignetteFile(h, config.VigBeforeTitle)
		if len(vignette) > 0 {
			div.AddNext("div", attr("class", "vignette_title_before")).
				AddNext("img", attr("src", path.Join("vignettes", filepath.Base(vignette))), attr("alt", config.VigBeforeTitle))
		}

		if err := p.transfer(from, div, "div", h); err != nil {
			return err
		}

		vignette = p.getVignetteFile(h, config.VigAfterTitle)
		if len(vignette) > 0 {
			div.AddNext("div", attr("class", "vignette_title_after")).
				AddNext("img", attr("src", path.Join("vignettes", filepath.Base(vignette))), attr("alt", config.VigAfterTitle))
		}

		p.Book.TOC = append(p.Book.TOC, &tocEntry{
			ref:      p.ctx().fname + "#" + tocRefID,
			title:    tocTitle,
			level:    p.ctx().header,
			bodyName: p.ctx().bodyName,
			main:     p.ctx().firstBodyTitle,
		})
	} else if err := p.transfer(from, to, "div", "titlenotes"); err != nil {
		return err
	}
	return nil
}

func transferSection(p *Processor, from, to *etree.Element) error {

	if len(p.ctx().bodyName) == 0 && p.ctx().header.Int() == 0 && p.ctx().firstBodyTitle {

		// processing section in main body, but there was no (body) title - we have to fake it to
		// keep structure uniform

		tocRefID := fmt.Sprintf("tocref%d", p.ctx().tocIndex)
		tocTitle := p.Book.BookAuthors(p.env.Cfg.Doc.AuthorFormat, true) + " " + p.Book.Title

		cls := "titleblock"
		if p.ctx().header.Int() >= p.env.Cfg.Doc.ChapterLevel {
			cls = "titleblock_nobreak"
		}
		div := to.AddNext("div", attr("id", tocRefID), attr("class", cls))

		h := p.ctx().header.String("h")
		vignette := p.getVignetteFile(h, config.VigBeforeTitle)
		if len(vignette) > 0 {
			div.AddNext("div", attr("class", "vignette_title_before")).
				AddNext("img", attr("src", path.Join("vignettes", filepath.Base(vignette))), attr("alt", config.VigBeforeTitle))
		}

		header := div.AddNext("div", attr("class", h))
		for _, an := range p.Book.Authors {
			header.AddNext("p", attr("class", "title")).SetText(ReplaceKeywords(p.env.Cfg.Doc.AuthorFormat, CreateAuthorKeywordsMap(an)))
		}
		header.AddNext("p", attr("class", "title")).SetText(p.Book.Title)

		vignette = p.getVignetteFile(h, config.VigAfterTitle)
		if len(vignette) > 0 {
			div.AddNext("div", attr("class", "vignette_title_after")).
				AddNext("img", attr("src", path.Join("vignettes", filepath.Base(vignette))), attr("alt", config.VigAfterTitle))
		}

		p.Book.TOC = append(p.Book.TOC, &tocEntry{
			ref:      p.ctx().fname + "#" + tocRefID,
			title:    tocTitle,
			level:    p.ctx().header,
			bodyName: p.ctx().bodyName,
			main:     p.ctx().firstBodyTitle,
		})

		p.ctx().firstBodyTitle = false
		p.ctx().tocIndex++
	}

	p.ctx().header.Inc()
	defer p.ctx().header.Dec()

	if len(p.ctx().bodyName) == 0 && p.ctx().header.Int() < p.env.Cfg.Doc.ChapterLevel {
		if p.env.Cfg.Doc.ChapterPerFile {
			// open next XHTML
			var f *dataFile
			to, f = p.ctx().createXHTML("")
			// store it for future flushing
			p.Book.Files = append(p.Book.Files, f)
			p.Book.Pages[f.fname] = 0
		}
	}

	// Since we are using recursive transfer algorithm when we return current file and other context values
	// will be quite different, so if we want to keep some values for this section we need another stack
	titler := p.ctx().sectionWithTitle.link()
	texter := p.ctx().sectionTextLength.link()
	if err := p.transfer(from, to, "div", "section"); err != nil {
		return err
	}
	hasTitle := titler()
	textLength := texter()

	if len(p.ctx().bodyName) == 0 {
		if textLength > 0 {
			if hasTitle {
				// only place vignette at the chapter end if it had it's own title and chapter has paragraphs with text
				vignette := p.getVignetteFile(p.ctx().header.String("h"), config.VigChapterEnd)
				if len(vignette) > 0 {
					to.AddNext("p", attr("class", "vignette_chapter_end")).
						AddNext("img", attr("src", path.Join("vignettes", filepath.Base(vignette))), attr("alt", config.VigChapterEnd))
				}
			} else if p.env.Cfg.Doc.TOC.NoTitleChapters {
				// section does not have a title - make sure TOC is not empty
				p.Book.TOC = append(p.Book.TOC, &tocEntry{
					ref:      p.ctx().fname + "#" + fmt.Sprintf("secref%d", p.ctx().findex),
					title:    fmt.Sprintf("%d", p.ctx().findex),
					level:    p.ctx().header,
					bodyName: p.ctx().bodyName,
				})
				p.ctx().tocIndex++
			}
		}
		to.AddNext("div", attr("class", "chapter_end"))
	}
	return nil
}

func transferImage(p *Processor, from, to *etree.Element) error {

	id := getAttrValue(from, "id")
	alt := getAttrValue(from, "alt")

	href := getAttrValue(from, "href")
	if len(href) > 0 {
		if u, err := url.Parse(href); err != nil {
			p.env.Log.Warn("unable to parse image ref-id", zap.String("href", href), zap.Error(err))
		} else {
			href = u.Fragment
		}
	}
	if len(href) == 0 {
		p.env.Log.Warn("Encountered image tag without href, skipping", zap.String("path", from.GetPath()), zap.String("xml", getXMLFragmentFromElement(from)))
		return nil
	}

	// find corresponding image
	var fname string
	for _, b := range p.Book.Images {
		if b.id == href {
			fname = b.fname
			break
		}
	}

	// oups
	if len(fname) == 0 {
		p.env.Log.Warn("Unable to find image for ref-id", zap.String("ref-id", href), zap.String("xml", getXMLFragmentFromElement(from)))
		var err error
		if p.notFound == nil {
			p.notFound, err = p.getNotFoundImage(len(p.Book.Images))
			if err != nil {
				return errors.Wrap(err, "unable to load not-found image")
			}
			p.Book.Images = append(p.Book.Images, p.notFound)
		}
		fname = p.notFound.fname
		alt = id
	}

	if len(alt) == 0 {
		alt = fname
	}

	out := to
	if p.ctx().inParagraph {
		if len(id) > 0 {
			out.AddNext("img", attr("id", id), attr("src", path.Join(DirImages, fname)), attr("alt", alt))
		} else {
			out.AddNext("img", attr("class", "inlineimage"), attr("src", path.Join(DirImages, fname)), attr("alt", alt))
		}
	} else {
		if len(id) > 0 {
			out = out.AddNext("div", attr("id", id), attr("class", "image"))
		} else {
			out = out.AddNext("div", attr("class", "image"))
		}
		out.AddNext("img", attr("src", path.Join(DirImages, fname)), attr("alt", alt))
	}
	return p.transfer(from, to)
}

func transferSpan(p *Processor, from, to *etree.Element) error {
	// allow span to keep all attributes
	attrs := make([]*etree.Attr, 0, 1)
	for _, a := range from.Attr {
		attrs = append(attrs, &etree.Attr{Space: a.Space, Key: a.Key, Value: a.Value})
	}
	return p.transfer(from, to.AddNext("span", attrs...))
}

func transferTable(p *Processor, from, to *etree.Element) error {
	attrs := make([]*etree.Attr, 0, 1)
	attrs = append(attrs, attr("class", "table"))
	for _, a := range from.Attr {
		attrs = append(attrs, &etree.Attr{Space: a.Space, Key: a.Key, Value: a.Value})
	}
	return p.transfer(from, to.AddNext("table", attrs...))
}

func transferTableElement(p *Processor, from, to *etree.Element) error {
	attrs := []*etree.Attr{}
	for _, a := range from.Attr {
		attrs = append(attrs, &etree.Attr{Space: a.Space, Key: a.Key, Value: a.Value})
	}
	return p.transfer(from, to.AddNext(from.Tag, attrs...))
}
