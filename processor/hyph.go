package processor

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/text/language"

	"github.com/rupor-github/fb2converter/hyphenator"
	"github.com/rupor-github/fb2converter/static"
)

type hyph struct {
	h *hyphenator.Hyphenator
}

// Some languages require additional specification.
var langMap = map[string]string{
	"de":    "de-1901",
	"de-de": "de-1901",
	"de-at": "de-1996",
	"de-ch": "de-ch-1901",
	"el":    "el-monoton",
	"el-gr": "el-monoton",
	"en":    "en-us",
	"mn":    "mn-cyrl",
	"sh":    "sh-latn",
	"sr":    "sr-cyrl",
	"zh":    "zh-latn-pinyin",
}

// newHyph loads hyphenation dictionary for specified language
func newHyph(lang language.Tag, log *zap.Logger) *hyph {

	// Let's hope this is enough
	names := []func(string) string{

		// language tag
		func(_ string) string {
			return strings.ToLower(lang.String())
		},

		// mapped language tag
		func(prev string) string {
			if name, ok := langMap[prev]; ok {
				return name
			}
			return ""
		},

		// base language tag
		func(_ string) string {
			b, c := lang.Base()
			if c != language.No {
				return strings.ToLower(b.String())
			}
			log.Debug("unable to find language base with at least some confidence", zap.Stringer("lang", lang), zap.Stringer("base", b))
			return ""
		},

		// mapped base language tag
		func(prev string) string {
			if name, ok := langMap[prev]; ok {
				return name
			}
			return ""
		},
	}

	var (
		dpat        []byte
		err         error
		lname, prev string
	)

	for i := 0; i < len(names); i++ {
		name := names[i](prev)
		if len(name) == 0 {
			continue
		}
		dpat, err = static.Asset(path.Join(DirHyphenator, fmt.Sprintf("hyph-%s.pat.txt", name)))
		if err != nil {
			prev = name
			continue
		}
		lname = name
		break
	}

	if len(lname) == 0 {
		log.Warn("Unable to find suitable hyphenation dictionary, turning off hyphenation", zap.Stringer("language", lang))
		return nil
	}

	dexc, err := static.Asset(path.Join(DirHyphenator, fmt.Sprintf("hyph-%s.hyp.txt", lname)))
	if err != nil {
		log.Warn("Unable to find suitable exceptions dictionary, leaving empty", zap.Stringer("language", lang))
	}

	h := &hyph{h: new(hyphenator.Hyphenator)}

	if err = h.h.LoadDictionary(lname, bytes.NewBuffer(dpat), bytes.NewBuffer(dexc)); err != nil {
		log.Warn("Unable to read hyphenation dictionary", zap.Stringer("language", lang), zap.Error(err))
		return nil
	}
	return h
}

// hyphenate inserts soft-hyphens.
func (h *hyph) hyphenate(in string) string {

	if h.h == nil {
		return in
	}
	return h.h.Hyphenate(in, strSOFTHYPHEN)
}
