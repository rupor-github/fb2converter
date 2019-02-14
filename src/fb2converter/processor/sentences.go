package processor

import (
	"fmt"
	"path"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"gopkg.in/neurosnap/sentences.v1"

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

// tokenize returns slice of sentences.
func (t *tokenizer) tokenize(in string) []string {

	var res []string
	if t.t == nil {
		return append(res, in)
	}

	for _, s := range t.t.Tokenize(in) {
		res = append(res, s.Text)
	}
	return res
}
