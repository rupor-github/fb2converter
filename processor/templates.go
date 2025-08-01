package processor

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	sprig "github.com/go-task/slim-sprig/v3"
)

type SequenceDefinition struct {
	Name   string
	Number int
}

type AuthorDefinition struct {
	FirstName, MiddleName, LastName string
}

type NoteDefinition struct {
	Name               string
	Number, NoteNumber int
}

// Values is a struct that holds variables we make available for template expansion
type Values struct {
	Context    string
	Title      string
	Series     SequenceDefinition
	Language   string
	Date       string
	Authors    []AuthorDefinition
	Format     string
	SourceFile string
	BookID     string
	ASIN       string
	Genres     []string
	// context dependent
	Author any
	Body   any
}

func (p *Processor) expandTemplate(name, field string, args ...any) (string, error) {

	funcMap := sprig.FuncMap()

	tmpl, err := template.New(name).Funcs(funcMap).Parse(field)
	if err != nil {
		return "", err
	}

	values := Values{
		Context: name,
		Title:   p.Book.Title,
		Series: SequenceDefinition{
			Name:   p.Book.SeqName,
			Number: p.Book.SeqNum,
		},
		Language:   p.Book.Lang.String(),
		Date:       p.Book.Date,
		Format:     p.format.String(),
		SourceFile: strings.TrimSuffix(filepath.Base(p.src), filepath.Ext(p.src)),
		BookID:     p.Book.ID.String(),
		ASIN:       p.Book.ASIN,
		Genres:     slices.Clone(p.Book.Genres),
	}

	for _, a := range p.Book.Authors {
		values.Authors = append(values.Authors, AuthorDefinition{
			FirstName:  a.First,
			MiddleName: a.Middle,
			LastName:   a.Last,
		})
	}

	// context dependent

	if strings.HasSuffix(name, "-author") {
		// find author definition in args
		if len(args) != 1 || args[0] == nil {
			return "", fmt.Errorf("author definition is required for %s template", name)
		}
		if author, ok := args[0].(AuthorDefinition); !ok {
			return "", fmt.Errorf("invalid author definition for %s template", name)
		} else {
			values.Author = author
		}
	}

	if strings.HasSuffix(name, "-notes") {
		// find note definition in args
		if len(args) != 1 || args[0] == nil {
			return "", fmt.Errorf("note definition is required for %s template", name)
		}
		if note, ok := args[0].(NoteDefinition); !ok {
			return "", fmt.Errorf("invalid note definition for %s template", name)
		} else {
			values.Body = note
		}
	}

	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, values); err != nil {
		return "", err
	}
	return buf.String(), nil
}
