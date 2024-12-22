package processor

import (
	"fmt"
	"path"
	"strings"
	"unicode"

	"github.com/neurosnap/sentences"
	"go.uber.org/zap"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"fb2converter/static"
)

type tokenizer struct {
	t *sentences.DefaultSentenceTokenizer
}

func newTokenizer(lang language.Tag, log *zap.Logger) *tokenizer {

	en := display.English.Languages()

	// Let's hope this is enough
	names := []func(string) string{

		// language tag
		func(_ string) string {
			return strings.ToLower(en.Name(lang))
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
		dpat, err = static.Asset(path.Join(DirSentences, fmt.Sprintf("%s.json", name)))
		if err != nil {
			prev = name
			continue
		}
		lname = name
		break
	}

	if len(lname) == 0 {
		log.Warn("Unable to find suitable sentences tokenizer data, using english", zap.Stringer("language", lang))
		dpat, err = static.Asset(path.Join(DirSentences, "english.json"))
		if err != nil {
			log.Warn("Unable to find english sentences tokenizer data, turning off sentences segmentation", zap.Error(err))
			return nil
		}
	}

	training, err := sentences.LoadTraining(dpat)
	if err != nil {
		log.Warn("Unable to load sentences tokenizer data, turning off sentences segmentation", zap.Error(err))
	}

	return &tokenizer{t: sentences.NewSentenceTokenizer(training)}
}

// splitSentences returns slice of sentences.
func splitSentences(t *tokenizer, in string) []string {

	var res []string
	if t == nil || t.t == nil {
		return append(res, in)
	}

	for _, s := range t.t.Tokenize(in) {
		res = append(res, s.Text)
	}

	// Sentences tokenizer has a funny way of working - sentence trailing spaces belong to the next sentence. That puts off
	// kepub viewer on Kobo devices. I do not want to change external "github.com/neurosnap/sentences" module - will do careful inplace
	// mockery right here instead.

	for i := 0; i < len(res)-1; i++ {
		for idx, sym := range res[i+1] {
			if !unicode.IsSpace(sym) {
				res[i] = res[i] + res[i+1][0:idx]
				res[i+1] = res[i+1][idx:]
				break
			}
		}
	}
	return res
}

// splitWords returns slice of words in sentence.
func splitWords(_ *tokenizer, in string, ignoreNBSP bool) []string {
	if ignoreNBSP {
		// unicode.IsSpace will eat everything - for backward compatibility
		return strings.Fields(in)
	}
	// exclude NBSP from the list of white space separators for latin1 symbols
	return strings.FieldsFunc(in, func(r rune) bool {
		if uint32(r) <= unicode.MaxLatin1 {
			switch r {
			case '\t', '\n', '\v', '\f', '\r', ' ', 0x85:
				return true
			}
			return false
		}
		return unicode.IsSpace(r)
	})
}
