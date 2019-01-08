package hyphenator

import (
	"bufio"
	"io"
	"strings"
	"text/scanner"
	"unicode/utf8"
)

// Hyphenator struct itself. The nil value is a hyphenator which has not been
// initialized with any hyphenation patterns or language yet.
type Hyphenator struct {
	patterns   *Trie
	exceptions map[string]string
	language   string
}

// LoadDictionary imports hyphenation patterns and exceptions from provided input streams.
func (h *Hyphenator) LoadDictionary(language string, patterns, exceptions io.Reader) error {

	if h.language != language {
		h.patterns = nil
		h.exceptions = nil
		h.language = language
	}

	if h.patterns != nil && h.patterns.Size() != 0 {
		// looks like it's already been set up
		return nil
	}

	h.patterns = NewTrie()
	h.exceptions = make(map[string]string, 20)

	if err := h.loadPatterns(patterns); err != nil {
		return err
	}
	if err := h.loadExceptions(exceptions); err != nil {
		return err
	}
	return nil
}

func (h *Hyphenator) loadPatterns(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		h.patterns.AddPatternString(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (h *Hyphenator) loadExceptions(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		str := scanner.Text()
		key := strings.Replace(str, `-`, ``, -1)
		h.exceptions[key] = str
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (h *Hyphenator) hyphenateWord(s, hyphen string) string {

	testStr := `.` + s + `.`
	v := make([]int, utf8.RuneCountInString(testStr))

	vIndex := 0
	for pos := range testStr {
		t := testStr[pos:]
		strs, values := h.patterns.AllSubstringsAndValues(t)
		for i := 0; i < len(values); i++ {
			str := strs[i]
			val := values[i].([]int)

			diff := len(val) - utf8.RuneCountInString(str)
			vs := v[vIndex-diff:]

			for i := 0; i < len(val); i++ {
				if val[i] > vs[i] {
					vs[i] = val[i]
				}
			}
		}
		vIndex++
	}

	var outstr string

	// trim the values for the beginning and ending dots
	markers := v[1 : len(v)-1]
	mIndex := 0
	u := make([]byte, 4)
	for _, ch := range s {
		l := utf8.EncodeRune(u, ch)
		outstr += string(u[0:l])
		// don't hyphenate between (or after) first two and the last two characters of a string
		if 1 <= mIndex && mIndex < len(markers)-2 {
			// hyphens are inserted on odd values, skipped on even ones
			if markers[mIndex]%2 != 0 {
				outstr += hyphen
			}
		}
		mIndex++
	}

	return outstr
}

// Hyphenate string.
func (h *Hyphenator) Hyphenate(s, hyphen string) string {

	var sc scanner.Scanner
	sc.Init(strings.NewReader(s))
	sc.Mode = scanner.ScanIdents
	sc.Whitespace = 0

	var outstr string

	tok := sc.Scan()
	for tok != scanner.EOF {
		switch tok {
		case scanner.Ident:
			// a word (or part thereof) to hyphenate
			t := sc.TokenText()

			// try the exceptions first
			exc := h.exceptions[t]
			if len(exc) != 0 {
				if hyphen != `-` {
					strings.Replace(exc, `-`, hyphen, -1)
				}
				return exc
			}

			// not an exception, hyphenate normally
			outstr += h.hyphenateWord(sc.TokenText(), hyphen)
		default:
			// A Unicode rune to append to the output
			p := make([]byte, utf8.UTFMax)
			l := utf8.EncodeRune(p, tok)
			outstr += string(p[0:l])
		}

		tok = sc.Scan()
	}
	return outstr
}
