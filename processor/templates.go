package processor

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	sprig "github.com/go-task/slim-sprig/v3"
)

type Sequence struct {
	Name   string
	Number int
}

type Author struct {
	FirstName, MiddleName, LastName string
}

// Values is a struct that holds variables we make available for template expansion
type Values struct {
	Context    string
	Title      string
	Series     Sequence
	Language   string
	Date       string
	Authors    []Author
	Format     string
	SourceFile string
	BookID     string
	ASIN       string
	Genres     []string
	// context dependent
	Author any
}

func (p *Processor) expandTemplate(name, field string, authorIndex int) (string, error) {

	funcMap := sprig.FuncMap()

	tmpl, err := template.New(name).Funcs(funcMap).Parse(field)
	if err != nil {
		return "", err
	}

	values := Values{
		Context: name,
		Title:   p.Book.Title,
		Series: Sequence{
			Name:   p.Book.SeqName,
			Number: p.Book.SeqNum,
		},
		Language:   p.Book.Lang.String(),
		Date:       p.Book.Date,
		Format:     p.format.String(),
		SourceFile: filepath.Base(p.src),
		BookID:     p.Book.ID.String(),
		ASIN:       p.Book.ASIN,
		Genres:     slices.Clone(p.Book.Genres),
	}

	processAuthor := strings.HasSuffix(name, "-author")
	for i, a := range p.Book.Authors {
		values.Authors = append(values.Authors, Author{
			FirstName:  a.First,
			MiddleName: a.Middle,
			LastName:   a.Last,
		})
		// context dependent
		if processAuthor && authorIndex == i {
			values.Author = Author{
				FirstName:  a.First,
				MiddleName: a.Middle,
				LastName:   a.Last,
			}
		}
	}

	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, values); err != nil {
		return "", err
	}
	return buf.String(), nil
}
