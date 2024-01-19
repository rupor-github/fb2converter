package processor

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/text/language"

	"fb2converter/config"
	"fb2converter/etree"
)

// TOC entries collected during parsing.
type tocEntry struct {
	ref      string
	title    string
	level    htmlHeader
	bodyName string
	main     bool
}

// Notes collected during parsing.
type note struct {
	title      string
	bodyName   string
	bodyNumber int
	body       string
	parsed     *etree.Element
}

// Links to notes collected.
type notelink struct {
	id       string
	bodyName string
}

// Book information and parsing context.
type Book struct {
	// description
	ID         uuid.UUID
	ASIN       string
	Title      string
	Lang       language.Tag
	Cover      string
	Genres     []string
	Authors    []*config.AuthorName
	SeqName    string
	SeqNum     int
	Annotation string
	Date       string
	// book structure
	TOC              []*tocEntry       // collected TOC entries
	Files            []*dataFile       // generated content
	Pages            map[string]int    // additional pages per file (file -> pages)
	Images           []*binImage       // parsed <binary> tags - book images
	Vignettes        []*binImage       // used vignette images
	LinksLocations   map[string]string // link ID -> file (in what file link id is)
	NoteBodyTitles   map[string]*note  // body name -> (note title, parsed title body)
	Notes            map[string]*note  // note ID -> (title, body)
	NotesParseOrder  []notelink        // notes in original order of parsing
	NotesBodyCount   int               // number of notes bodies parsed
	NotesUsageOrder  []string          // notes in order of usage (when encountered)
	NotesPerBodyUsed map[string]int    // number of notes per body as encountered on use
	Data             []*dataFile       // various files: stylesheet, fonts...
	Meta             []*dataFile       // container meta-info
	// parsing context
	context      *context
	contextStack []*context
	hyph         *hyph
	tokenizer    *tokenizer
}

// NewBook returns pointer to book.
func NewBook(u uuid.UUID, name string) *Book {
	return &Book{
		ID:               u,
		Title:            name,
		Lang:             language.Russian,
		Pages:            make(map[string]int),
		LinksLocations:   make(map[string]string),
		NoteBodyTitles:   make(map[string]*note),
		Notes:            make(map[string]*note),
		NotesPerBodyUsed: make(map[string]int),
		context:          newContext(),
	}
}

// ReorderNotes reorders NotesOrder in accordance with NotesUsageOrder.
func (b *Book) ReorderNotes(log *zap.Logger) bool {

	// sanity check
	if len(b.NotesParseOrder) == 0 || len(b.Notes) == 0 {
		return false
	}

	// create map of notes order
	npo := make(map[string]notelink, len(b.NotesParseOrder))
	for _, nl := range b.NotesParseOrder {
		npo[nl.id] = nl
	}

	// reorder everything using usage order
	nuo := make(map[string]bool, len(b.NotesUsageOrder))
	notes := make([]notelink, 0, len(b.NotesUsageOrder))
	for _, id := range b.NotesUsageOrder {
		var (
			nl notelink
			ok bool
		)
		if nl, ok = npo[id]; !ok {
			log.Debug("Unable to locate used note link in notes order", zap.String("id", id))
			return false
		}
		if _, ok := nuo[id]; !ok {
			notes = append(notes, nl)
			nuo[id] = ok
		}
	}

	log.Debug("Reordering notes data",
		zap.Int("Number of notes", len(b.Notes)),
		zap.Int("Number of notes links before", len(b.NotesParseOrder)),
		zap.Int("Number of notes links after", len(notes)),
		zap.Int("Number of anchors", len(b.NotesUsageOrder)),
	)

	b.NotesParseOrder = notes
	return true
}

// BookAuthors returns authors as a single string.
func (b *Book) BookAuthors(format string, short bool) string {
	if len(b.Authors) == 0 {
		return ""
	}
	if short && len(b.Authors) > 1 {
		if b.Lang == language.Russian {
			return ReplaceKeywords(format, CreateAuthorKeywordsMap(b.Authors[0])) + " и др"
		}
		return ReplaceKeywords(format, CreateAuthorKeywordsMap(b.Authors[0])) + ", et al"
	}
	res := make([]string, 0, len(b.Authors))
	for _, an := range b.Authors {
		res = append(res, ReplaceKeywords(format, CreateAuthorKeywordsMap(an)))
	}
	return strings.Join(res, ", ")
}

// flushMeta saves all container meta files.
func (b *Book) flushMeta(path string) error {
	for _, f := range b.Meta {
		if err := f.flush(path); err != nil {
			return err // no point continuing
		}
	}
	return nil
}

// flushData saves all "data" files.
func (b *Book) flushData(path string) error {

	if len(b.Data) == 0 {
		return nil
	} else if len(b.Data) == 1 {
		if err := b.Data[0].flush(path); err != nil {
			return err // no point continuing
		}
		return nil
	}

	var (
		haveError int32
		wg        sync.WaitGroup
	)

	job := make(chan *dataFile, len(b.Data))
	res := make(chan error, len(b.Data))

	// start processing pool
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func(job <-chan *dataFile, res chan<- error) {
			defer wg.Done()
			for f := range job {
				if f == nil || atomic.LoadInt32(&haveError) != 0 {
					break
				}
				err := f.flush(path)
				if err != nil {
					atomic.AddInt32(&haveError, 1)
					res <- err
					break
				}
			}
		}(job, res)
	}

	// supply work
	for _, f := range b.Data {
		if atomic.LoadInt32(&haveError) != 0 {
			break
		}
		job <- f
	}
	close(job)
	wg.Wait()

	if haveError != 0 {
		// return first error
		return <-res
	}
	return nil
}

// flushXHTML saves all content files generated by transforming fb2.
func (b *Book) flushXHTML(path string) error {

	if len(b.Files) == 0 {
		return nil
	} else if len(b.Files) == 1 {
		if err := b.Files[0].flush(path); err != nil {
			return err // no point continuing
		}
		return nil
	}

	var (
		haveError int32
		wg        sync.WaitGroup
	)

	job := make(chan *dataFile, len(b.Files))
	res := make(chan error, len(b.Files))

	// start processing pool
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func(job <-chan *dataFile, res chan<- error) {
			defer wg.Done()
			for f := range job {
				if f == nil || atomic.LoadInt32(&haveError) != 0 {
					break
				}
				err := f.flush(path)
				if err != nil {
					atomic.AddInt32(&haveError, 1)
					res <- err
					break
				}
			}
		}(job, res)
	}

	// supply work
	for _, f := range b.Files {
		if atomic.LoadInt32(&haveError) != 0 {
			break
		}
		job <- f
	}
	close(job)
	wg.Wait()

	if haveError != 0 {
		// return first error
		return <-res
	}
	return nil
}

// flushImages saves all images - coming from fb2 binary tags.
func (b *Book) flushImages(path string) error {

	if len(b.Images) == 0 {
		return nil
	}

	if len(b.Images) == 1 {
		if err := b.Images[0].flush(path); err != nil {
			return err // no point continuing
		}
		return nil
	}

	var (
		haveError int32
		wg        sync.WaitGroup
	)

	job := make(chan *binImage, len(b.Images))
	res := make(chan error, len(b.Images))

	// start processing pool
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func(job <-chan *binImage, res chan<- error) {
			defer wg.Done()
			for f := range job {
				if f == nil || atomic.LoadInt32(&haveError) != 0 {
					break
				}
				err := f.flush(path)
				if err != nil {
					atomic.AddInt32(&haveError, 1)
					res <- err
					break
				}
			}
		}(job, res)
	}

	// supply work
	for _, f := range b.Images {
		if atomic.LoadInt32(&haveError) != 0 {
			break
		}
		job <- f
	}
	close(job)
	wg.Wait()

	if haveError != 0 {
		// return first error
		return <-res
	}
	return nil
}

// flushVignettes saves all vignettes used for content.
func (b *Book) flushVignettes(path string) error {

	if len(b.Vignettes) == 0 {
		return nil
	}

	for _, f := range b.Vignettes {
		if err := f.flush(path); err != nil {
			return err // no point continuing
		}
	}
	return nil
}

// ctx returns current context.
func (b *Book) ctx() *context {
	return b.context
}

// ctxPush pushes current context on the stack, creates new context (empty), makes it current and returns it.
func (b *Book) ctxPush() *context {
	b.contextStack = append(b.contextStack, b.context)
	b.context = newContext()
	return b.context
}

// ctxPop pops context from the stack and makes it current. Old "current" context is returned.
func (b *Book) ctxPop() *context {
	cur := b.context
	b.context = b.contextStack[len(b.contextStack)-1]
	b.contextStack[len(b.contextStack)-1] = nil
	b.contextStack = b.contextStack[:len(b.contextStack)-1]
	return cur
}
