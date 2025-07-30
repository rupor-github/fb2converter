package processor

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"go.uber.org/zap"

	"fb2converter/config"
	toml "fb2converter/go-micro/config/encoder/toml"
	"fb2converter/state"
)

type testCaseTmpl struct {
	context string
	arg     any
	tmpl    []byte
	result  string
}

const titleTemplate = `template = """\
{{- /* Shorten book authors + book title */ -}}\
{{- if hasPrefix "section-" .Context -}}\
{{-   $all := "" -}}\
{{-   if gt (len .Authors) 0 -}}\
{{-     with first .Authors -}}\
{{-       $all = .LastName -}}\
{{-       if .FirstName }}{{ $all = (cat $all .FirstName) }}{{- end -}}\
{{-       if .MiddleName }}{{ $all = (cat $all .MiddleName) }}{{- end -}}\
{{-     end -}}\
{{-     if gt (len .Authors) 1 -}}\
{{-       if eq .Language "ru" }}{{ $all = (cat $all "и др") }}{{- else -}}{{ $all = (printf "%s%s" $all ", et al") }}{{- end -}}\
{{-     end -}}\
{{-   end -}}\
{{-   cat $all .Title -}}\
{{- /* Shorten book authors */ -}}\
{{- else if hasPrefix "stamp-" .Context -}}\
{{-   if gt (len .Authors) 0 -}}\
{{-     $all := "" -}}\
{{-     with first .Authors -}}\
{{-       $all = .LastName }}\
{{-       if .FirstName }}{{ $all = (cat $all .FirstName) }}{{- end -}}\
{{-       if .MiddleName }}{{ $all = (cat $all .MiddleName) }}{{- end -}}\
{{-     end -}}\
{{-     if gt (len .Authors) 1 -}}\
{{-       if eq .Language "ru" }}{{ $all = (cat $all "и др") }}{{- else -}}{{ $all = (printf "%s%s" $all ", et al") }}{{- end -}}\
{{-     end -}}\
{{-     $all -}}
{{-   end -}}\
{{- /* All book authors */ -}}\
{{- else if hasPrefix "toc-" .Context -}}\
{{-   $all := list -}}\
{{-   range .Authors -}}\
{{-     $name := .LastName -}}\
{{-     if .FirstName -}}{{ $name = (cat $name .FirstName) -}}{{- end -}}\
{{-     if .MiddleName -}}{{ $name = (cat $name .MiddleName) -}}{{- end -}}\
{{-     $all = append $all $name -}}\
{{-   end -}}\
{{-   join ", " $all -}}\
{{- /* Abbreviated series/number + book title */ -}}\
{{- else -}}\
{{-   $seriesLetters := list -}}\
{{-   range $word := splitList " " .Series.Name -}}\
{{-     $seriesLetters = append $seriesLetters (upper (first (splitList "" $word))) -}}\
{{-   end -}}\
{{-   if gt (len $seriesLetters) 0 -}}\
{{-     "(" }}{{- join "" $seriesLetters -}}\
{{-     if gt .Series.Number 0 -}}\
{{-       printf " - %02d" .Series.Number -}}\
{{-     end -}}\
{{-     ") " -}}\
{{-   end -}}\
{{    .Title -}}\
{{- end -}}\
"""`

const filenameTemplate = `template = """\
{{- $all := "" -}}\
{{- if gt (len .Authors) 0 -}}\
{{-   with first .Authors -}}\
{{-     $all = .LastName -}}\
{{-     if .FirstName }}{{ $all = (cat $all .FirstName) }}{{- end -}}\
{{-     if .MiddleName }}{{ $all = (cat $all .MiddleName) }}{{- end -}}\
{{-   end -}}\
{{-   if gt (len .Authors) 1 -}}\
{{-     if eq .Language "ru" }}{{ $all = (cat $all "и др") }}{{- else -}}{{ $all = (printf "%s%s" $all ", et al") }}{{- end -}}\
{{-   end -}}\
{{-   $all = cat $all "-" -}}\
{{- end -}}\
{{- if $all -}}
{{-   cat $all .Title -}}\
{{- else -}}\
{{-   .Title -}}\
{{- end -}}\
"""`

const linkNoteTemplate = `template = """\
	[{{- if gt .Body.Number 0 -}}
	 {{-   printf "%d" .Body.Number -}}.\
	 {{- end -}}\
	 {{- printf "%d" .Body.NoteNumber -}}]\
"""`

var tests = []testCaseTmpl{
	{
		context: "section-author",
		arg:     AuthorDefinition{FirstName: "Иван", MiddleName: "Иванович", LastName: "Иванов"},
		tmpl: []byte(`template = """\
				{{- with .Author }}\
				{{   .LastName }}\
				{{   if .FirstName }} {{ .FirstName}}{{ end }}\
				{{   if .MiddleName }} {{ .MiddleName }}{{ end }}\
			  {{ end -}}\
		      """`),
		result: "Иванов Иван Иванович",
	},
	{
		context: "opf-title",
		tmpl:    []byte(titleTemplate),
		result:  "(Т - 01) Тестовая книга",
	},
	{
		context: "section-title",
		tmpl:    []byte(titleTemplate),
		result:  "Иванов Иван Иванович и др Тестовая книга",
	},
	{
		context: "stamp-title",
		tmpl:    []byte(titleTemplate),
		result:  "Иванов Иван Иванович и др",
	},
	{
		context: "toc-title",
		tmpl:    []byte(titleTemplate),
		result:  "Иванов Иван Иванович, Петров Пётр Петрович, Сидоров Сидор Сидорович",
	},
	{
		context: "output-filename",
		tmpl:    []byte(filenameTemplate),
		result:  "Иванов Иван Иванович и др - Тестовая книга",
	},
	{
		context: "link-notes",
		arg:     NoteDefinition{Name: "n_17", Number: 0, NoteNumber: 17},
		tmpl:    []byte(linkNoteTemplate),
		result:  "[17]",
	},
	{
		context: "link-notes",
		arg:     NoteDefinition{Name: "n_17", Number: 22, NoteNumber: 17},
		tmpl:    []byte(linkNoteTemplate),
		result:  "[22.17]",
	},
	{
		context: "output-filename",
		tmpl: []byte(`template = """\
					{{- $parts := list -}}\
					{{- if gt (len .Authors) 0 -}}\
					{{-   $name := "" -}}\
					{{-   with first .Authors -}}\
					{{-     $name = .LastName -}}\
					{{-     if .FirstName }}{{ $name = (cat $name .FirstName) }}{{- end -}}\
					{{-     if .MiddleName }}{{ $name = (cat $name .MiddleName) }}{{- end -}}\
					{{-   end -}}\
					{{-   $parts = append $parts $name -}}\
					{{- end -}}\
					{{- if .Series.Name -}}\
					{{-   $parts = append $parts .Series.Name -}}\
					{{- else -}}\
					{{-   $parts = append $parts "-" -}}\
					{{- end -}}\
					{{- $last := "" -}}\
					{{- if gt (len .Authors) 0 -}}\
					{{-   with first .Authors -}}\
					{{-     $last = .LastName -}}\
					{{-   end -}}\
					{{- end -}}\
					{{- if gt .Series.Number 0 -}}\
					{{-   $last = printf "%s %02d" $last .Series.Number -}}\
					{{- end -}}\
					{{- $last = (cat $last .Title) -}}\
					{{- $parts = append $parts $last -}}\
					{{- join "/" $parts -}}\
	            """`),
		result: "Иванов Иван Иванович/Тесты/Иванов 01 Тестовая книга",
	},
}

func TestTemplates(t *testing.T) {

	f, err := os.CreateTemp("", "fb2-test-config-*.toml")
	if err != nil {
		t.Fatalf("Unable to create temporary config file: %v", err)
	}

	cfgName := f.Name()

	_, err = io.Copy(f, bytes.NewReader(testConfig))
	if err != nil {
		t.Fatalf("Unable to write temporary config file: %v", err)
	}
	f.Close()

	// log, err := zap.NewDevelopment()
	// if err != nil {
	// 	t.Fatal(err)
	// }
	log := zap.NewNop()

	cfg, err := config.BuildConfig(cfgName)
	if err != nil {
		t.Fatal(err)
	}

	env := &state.LocalEnv{
		Log: log,
		Cfg: cfg,
	}

	p, err := NewFB2(bytes.NewReader(fb2), false, "path1/path2/file.fb2", "/path3/path4", false, false, false, OEpub, env)
	if err != nil {
		t.Fatalf("Unable to parse book: %v", err)
	}

	err = p.Process()
	if err != nil {
		t.Fatalf("Unable to process book: %v", err)
	}

	for i, c := range tests {
		enc := toml.NewEncoder()
		var v map[string]string
		if err := enc.Decode(c.tmpl, &v); err != nil {
			t.Fatal(err)
		}
		tmpl, ok := v["template"]
		if !ok {
			t.Fatalf("Bad decoding %d: %s", i, spew.Sprint(v))
		}
		res, err := p.expandTemplate(c.context, tmpl, c.arg)
		if err != nil {
			t.Fatal(err, c.arg, tmpl)
		}
		if res != c.result {
			t.Fatalf("Unexpected for %d: [%s] != [%s]", i, res, c.result)
		}
	}
}

var testConfig = []byte(`
[document]
	version = 2
`)

var fb2 = []byte(`<?xml version="1.0" encoding="utf-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0" xmlns:l="http://www.w3.org/1999/xlink">

 <description>
  <title-info>
   <genre>det_classic</genre>
   <author>
    <first-name>Иван</first-name>
    <middle-name>Иванович</middle-name>
    <last-name>Иванов</last-name>
   </author>
   <author>
    <first-name>Пётр</first-name>
    <middle-name>Петрович</middle-name>
    <last-name>Петров</last-name>
   </author>
   <author>
    <first-name>Сидор</first-name>
    <middle-name>Сидорович</middle-name>
    <last-name>Сидоров</last-name>
   </author>
   <book-title>Тестовая книга</book-title>
   <annotation>
    <p>Здесь полагается быть <strong>краткому</strong>, но весьма <strong>живописному</strong> описанию<sup><a l:href="#n_17" type="note">[17]</a></sup> предлагаемой читателю книги.</p>
    <p>Как правило, с действительностью ничего общего не имеет. Чистая реклама в классическом стиле – «Вам сегодня необыкновенно повезло!»</p>
    <empty-line/>
    <p>Для разнообразия в этой аннотации будет указано истинное назначение книги – тестирование возможности корректной конвертации формата FB2 в любой другой формат, понимаемый прежде всего электронными книгами (eBooks).</p>
    <p>В данной тестовой книге используется различное стилевое оформление текста, сноски, стихотворения, таблицы, изображения (в том числе с прозрачностью) и т.п.</p>
    <p>Книга <strong>не</strong> является полностью валидной с точки зрения наиболее распространённых FB2-редакторов. К сожалению, количество косяков в существующих FB2-книгах превосходит все разумные пределы, и это приходится учитывать.</p>
    <p>Свои пожелания по добавлению категорически необходимых плюшек можно отправлять на форум «The-eBook» в личку <strong><a l:href="http://www.the-ebook.org/forum/profile.php?mode=viewprofile&amp;u=9389">старику Кацу</a></strong>.</p>
   </annotation>
   <date>26.10.2015</date>
   <coverpage>
    <image l:href="#cover.jpg"/>
   </coverpage>
   <lang>ru</lang>
   <src-lang>ru</src-lang>
   <sequence name="Тесты" number="1"/>
  </title-info>
  <document-info>
   <author>
    <nickname>kaznelson</nickname>
   </author>
   <program-used>Book Designer 5.0, FictionBook Editor Release 2.6, AkelPad 4</program-used>
   <date value="201-105-26">26.10.2015</date>
   <id>BD-A2B81E-20EB-3A4E-EE8F-1664-26E3-BEF744</id>
   <version>2.4</version>
  </document-info>
 </description>

 <body>
  <title>
   <p>Авторский коллектив</p>
   <empty-line/>
   <p>Тестовый FB2</p>
  </title>

  <section id="s_1">
   <title>
    <p>Эпиграф.</p>
    <p>Текст.<sup><a l:href="#n_18" type="note">[18]</a></sup></p>
    <p>Ударения.</p>
   </title>

   <epigraph>
    <p>Спортсмены! Принимая низкий старт, убедитесь, что сзади никто не бежит с шестом!</p>
    <text-author>Автор<sup><a l:href="#n_1" type="note">[1]</a></sup></text-author>
   </epigraph>

   <empty-line/>
   <subtitle><emphasis>Перекрёстные ссылки</emphasis></subtitle>
   <p><emphasis><a l:href="#s_2">Глава 2</a></emphasis></p>
   <empty-line/>
   <p id="p_1">Выше дана перекрёстная ссылка на секцию с заголовком (<emphasis>&lt;section id="s_2"&gt;</emphasis> + <emphasis>&lt;title&gt;</emphasis>).</p>
   <p>Пример ссылки между параграфами (<emphasis>&lt;p id="p_2"&gt;</emphasis>), ведущий в последний параграф этой главы: <emphasis>[<a l:href="#p_2">перекрёстная ссылка</a>]</emphasis>.</p>
   <p>Пример ссылки между параграфами, ведущий в сноску главы «Надстрочник и подстрочник»: <emphasis>[<a l:href="#p_3">перекрёстная ссылка</a>]</emphasis>.</p>
   <empty-line/>
  </section>
 </body>
</FictionBook>
`)
