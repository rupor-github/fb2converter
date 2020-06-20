package processor

import (
	"crypto/md5"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rupor-github/fb2converter/config"
)

const (
	strNBSP       = "\u00A0"
	strSOFTHYPHEN = "\u00AD"
)

type htmlHeader int

func (hl *htmlHeader) Inc() {
	if *hl < math.MaxInt32 {
		*hl++
	}
}

func (hl *htmlHeader) Dec() {
	if *hl > 0 {
		*hl--
	}
}

func (hl htmlHeader) Int() int {
	return int(hl)
}

func (hl htmlHeader) String(prefix string) string {
	if hl > 6 {
		hl = 6
	}
	return fmt.Sprintf("%s%d", prefix, hl)
}

// GetFirstRuneString returns first UTF-8 rune of the passed in string.
func GetFirstRuneString(in string) string {
	for _, c := range in {
		return string(c)
	}
	return ""
}

// GenSafeName takes a string and generates file name form it which is safe to use everywhere.
func GenSafeName(name string) string {
	h := md5.New()
	_, _ = io.WriteString(h, name)
	return fmt.Sprintf("zz%x", h.Sum(nil))
}

var nameCleaner = strings.NewReplacer("\r", "", "\n", "", " ", "")

// SanitizeName in case name needs cleanup.
func SanitizeName(in string) (out string, changed bool) {
	out = nameCleaner.Replace(in)
	return out, out != in
}

var noteCleaner = regexp.MustCompile(`[\[{].*[\]}]`)

// SanitizeTitle removes footnote leftovers and CR (in case this is Windows).
func SanitizeTitle(in string) string {
	return strings.Replace(noteCleaner.ReplaceAllLiteralString(in, ""), "\r", "", -1)
}

// AllLines joins lines using space as a EOL replacement.
func AllLines(in string) string {
	return strings.Join(strings.Split(in, "\n"), " ")
}

// FirstLine returns first line for supplied string.
func FirstLine(in string) string {
	return strings.Split(in, "\n")[0]
}

// ReplaceKeywords scans provided string for keys from the map and replaces them with corresponding values from the map.
// Curly brackets '{' and '}' are special - they indicate conditional block. If all keys inside block were replaced with
// empty values - whole block inside curly brackets will be removed. Blocks could be nested. Curly brackets could be escaped
// with backslash if necessary.
func ReplaceKeywords(in string, m map[string]string) string {

	expandKeyword := func(in, key, value string) (string, bool) {
		if strings.Contains(in, key) {
			return strings.Replace(in, key, value, -1), len(value) > 0
		}
		return in, false
	}

	expandAll := func(in string, m map[string]string) string {

		// NOTE: to provide stable results longer keywords should be replaced first (#authors then #author)
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var expanded, ok bool
		for i := len(keys) - 1; i >= 0; i-- {
			in, ok = expandKeyword(in, keys[i], m[keys[i]])
			expanded = expanded || ok
		}
		if !expanded {
			return ""
		}
		return in
	}

	bopen, bclose := -1, -1

	// I do not want to write real parser
	in = strings.Replace(strings.Replace(in, `\{`, "\x01", -1), `\}`, "\x02", -1)

	for i, c := range in {
		if c == '{' {
			bopen = i
		} else if c == '}' {
			bclose = i
			break
		}
	}

	var out string
	if bopen >= 0 && bclose > 0 && bopen < bclose {
		out = ReplaceKeywords(in[:bopen]+expandAll(in[bopen+1:bclose], m)+in[bclose+1:], m)
	} else {
		out = expandAll(in, m)
	}
	return strings.Replace(strings.Replace(out, "\x01", "{", -1), "\x02", "}", -1)
}

// CreateAuthorKeywordsMap prepares keywords map for replacement.
func CreateAuthorKeywordsMap(an *config.AuthorName) map[string]string {
	rd := make(map[string]string)
	if len(an.First) > 0 {
		rd["#f"], rd["#fi"] = an.First, GetFirstRuneString(an.First)+"."
	} else {
		rd["#f"], rd["#fi"] = "", ""
	}
	if len(an.Middle) > 0 {
		rd["#m"], rd["#mi"] = an.Middle, GetFirstRuneString(an.Middle)+"."
	} else {
		rd["#m"], rd["#mi"] = "", ""
	}
	if len(an.Last) > 0 {
		rd["#l"] = an.Last
	} else {
		rd["#l"] = ""
	}
	return rd
}

// CreateTitleKeywordsMap prepares keywords map for replacement.
func CreateTitleKeywordsMap(b *Book, pos int, src string) map[string]string {
	rd := make(map[string]string)
	rd["#title"] = ""
	if len(b.Title) > 0 {
		rd["#title"] = b.Title
	}
	base := filepath.Base(src)
	if len(base) > 1 {
		rd["#file_name"], rd["#file_name_ext"] = strings.TrimSuffix(base, filepath.Ext(base)), base
	}
	rd["#series"], rd["#abbrseries"], rd["#ABBRseries"] = "", "", ""
	if len(b.SeqName) > 0 {
		rd["#series"] = b.SeqName
		abbr := abbrSeq(b.SeqName)
		if len(abbr) > 0 {
			rd["#abbrseries"] = strings.ToLower(abbr)
			rd["#ABBRseries"] = strings.ToUpper(abbr)
		}
	}
	rd["#number"], rd["#padnumber"] = "", ""
	if b.SeqNum > 0 {
		rd["#number"] = fmt.Sprintf("%d", b.SeqNum)
		rd["#padnumber"] = fmt.Sprintf(fmt.Sprintf("%%0%dd", pos), b.SeqNum)
	}
	rd["#date"] = ""
	if len(b.Date) > 0 {
		rd["#date"] = b.Date
	}
	return rd
}

func abbrSeq(seq string) (abbr string) {
	for _, w := range strings.Split(seq, " ") {
		for len(w) > 0 {
			r, l := utf8.DecodeRuneInString(w)
			if r != utf8.RuneError && unicode.IsLetter(r) {
				abbr += string(r)
				break
			}
			w = w[l:]
		}
	}
	return
}

// CreateFileNameKeywordsMap prepares keywords map for replacement.
func CreateFileNameKeywordsMap(b *Book, format string, pos int) map[string]string {
	rd := make(map[string]string)
	rd["#title"] = ""
	if len(b.Title) > 0 {
		rd["#title"] = b.Title
	}
	rd["#series"], rd["#abbrseries"], rd["#ABBRseries"] = "", "", ""
	if len(b.SeqName) > 0 {
		rd["#series"] = b.SeqName
		abbr := abbrSeq(b.SeqName)
		if len(abbr) > 0 {
			rd["#abbrseries"] = strings.ToLower(abbr)
			rd["#ABBRseries"] = strings.ToUpper(abbr)
		}
	}
	rd["#number"], rd["#padnumber"] = "", ""
	if b.SeqNum > 0 {
		rd["#number"] = fmt.Sprintf("%d", b.SeqNum)
		rd["#padnumber"] = fmt.Sprintf(fmt.Sprintf("%%0%dd", pos), b.SeqNum)
	}
	rd["#authors"] = b.BookAuthors(format, false)
	rd["#author"] = b.BookAuthors(format, true)
	rd["#bookid"] = b.ID.String()
	return rd
}

// CreateAnchorLinkKeywordsMap prepares keywords map for replacement.
func CreateAnchorLinkKeywordsMap(name string, bodyNumber, noteNumber int) map[string]string {
	rd := make(map[string]string)
	rd["#number"] = strconv.Itoa(noteNumber)
	if bodyNumber > 0 {
		rd["#body_number"] = strconv.Itoa(bodyNumber)
	}
	rd["#body_name"] = name

	fl := GetFirstRuneString(name)

	rd["#body_name_Fl"] = fl
	rd["#body_name_fl"] = strings.ToLower(fl)
	rd["#body_name_FL"] = strings.ToUpper(fl)

	return rd
}

// AppendIfMissing well append string to slice only if it is not there already.
func AppendIfMissing(slice []string, str string) []string {
	for _, s := range slice {
		if s == str {
			return slice
		}
	}
	return append(slice, str)
}

// IsOneOf checks if string is present in slice of strings. Comparison is case insensitive.
func IsOneOf(name string, names []string) bool {
	for _, n := range names {
		if strings.EqualFold(name, n) {
			return true
		}
	}
	return false
}
