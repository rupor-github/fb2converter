package processor

import (
	"net/url"
	"strings"
	"testing"
)

type testCase struct {
	in  string
	m   map[string]string
	out string
}

var cases = []testCase{
	{
		in: `#l #f #m`,
		m: map[string]string{
			"#l": "Last_Name",
			"#f": "First_Name",
			"#m": "Middle_Name",
		},
		out: `Last_Name First_Name Middle_Name`,
	},
	{
		in: `#l #f{ #m}`,
		m: map[string]string{
			"#l": "Last_Name",
			"#f": "First_Name",
			"#m": "Middle_Name",
		},
		out: `Last_Name First_Name Middle_Name`,
	},
	{
		in: `#l #f{ #m}`,
		m: map[string]string{
			"#l": "Last_Name",
			"#f": "First_Name",
			"#m": "",
		},
		out: `Last_Name First_Name`,
	},
	{
		in: `#l #f{ aaaaaaaaaa }`,
		m: map[string]string{
			"#l": "Last_Name",
			"#f": "First_Name",
			"#m": "Middle_Name",
		},
		out: `Last_Name First_Name`,
	},
	{
		in: `#l{ #f{ #m}}`,
		m: map[string]string{
			"#l": "Last_Name",
			"#f": "First_Name",
			"#m": "",
		},
		out: `Last_Name First_Name`,
	},
	{
		in: `#l{ \{mm\} #f{ #m}}`,
		m: map[string]string{
			"#l": "Last_Name",
			"#f": "First_Name",
			"#m": "",
		},
		out: `Last_Name {mm} First_Name`,
	},
	{
		in: `#authors #title #author`,
		m: map[string]string{
			"#author":  "_single_author_",
			"#authors": "_multiple_authors_",
			"#title":   "book-title",
		},
		out: `_multiple_authors_ book-title _single_author_`,
	},
	{
		in: `#abbrseries #ABBRseries`,
		m: map[string]string{
			"#abbrseries": "_a_b_c_",
			"#ABBRseries": "_A_B_C_",
		},
		out: `_a_b_c_ _A_B_C_`,
	},
}

func TestReplaceKeywords(t *testing.T) {

	for i, c := range cases {
		res := ReplaceKeywords(c.in, c.m)
		if res != c.out {
			t.Fatalf("BAD RESULT for case %d\nEXPECTED:\n[%s]\nGOT:\n[%s]", i+1, c.out, res)
		}
	}
	t.Logf("OK - %s: %d cases", t.Name(), len(cases))
}

var cases1 = []string{
	"1",
	"test book.epub",
	"Знаменитые расследования Мисс Марпл в одном томе .epub",
}

func TestContentDisposition(t *testing.T) {
	for i, c := range cases1 {
		res1 := url.PathEscape(c)
		res2 := ""
		for _, part := range encodeParts(c) {
			res2 += strings.TrimPrefix(part, rfc8187charset)
		}
		if res1 != res2 {
			t.Fatalf("BAD RESULT for case %d [%s]\nEXPECTED:\n[%s]\nGOT:\n[%s]", i+1, c, res1, res2)
		}
	}
	t.Logf("OK - %s: %d cases", t.Name(), len(cases1))
}
