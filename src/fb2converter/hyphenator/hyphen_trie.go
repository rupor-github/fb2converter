package hyphenator

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// AddPatternString specialized function for TeX-style hyphenation patterns.  Accepts strings of the form '.hy2p'.
// The value it stores is of type []int
func (p *Trie) AddPatternString(s string) {

	v := []int{}

	// precompute the Unicode rune for the character '0'
	zero, _ := utf8.DecodeRune([]byte{'0'})

	strLen := utf8.RuneCountInString(s)

	// Using the range keyword will give us each Unicode rune.
	for pos, sym := range s {

		if unicode.IsDigit(sym) {
			if pos == 0 {
				// This is a prefix number
				v = append(v, int(sym-zero))
			}
			// this is a number referring to the previous character, and has
			// already been handled
			continue
		}

		if pos < strLen-1 {
			// look ahead to see if it's followed by a number
			next := []rune(s)[pos+1]
			if unicode.IsDigit(next) {
				// next char is the hyphenation value for this char
				v = append(v, int(next-zero))
			} else {
				// hyphenation for this char is an implied zero
				v = append(v, 0)
			}
		} else {
			// last character gets an implied zero
			v = append(v, 0)
		}
	}

	pure := strings.Map(func(sym rune) rune {
		if unicode.IsDigit(sym) {
			return -1
		}
		return sym
	}, s)

	leaf := p.addRunes(strings.NewReader(pure))
	if leaf == nil {
		return
	}
	leaf.value = v
}
