package hyphenator

import (
	"fmt"
	"os"
	"testing"
)

func buildHyphenator(t *testing.T, lang string) *Hyphenator {
	fpat, err := os.Open(fmt.Sprintf("../static/dictionaries/hyph-%s.pat.txt", lang))
	if err != nil {
		t.Fatal("Unable to open hyph-en-us.pat.txt", err)
	}
	defer fpat.Close()

	fexc, err := os.Open(fmt.Sprintf("../static/dictionaries/hyph-%s.hyp.txt", lang))
	if err != nil {
		t.Fatal("Unable to open hyph-en-us.hyp.txt", err)
	}
	defer fexc.Close()

	h := new(Hyphenator)
	h.LoadDictionary(lang, fpat, fexc)
	return h
}

// technically this string contains an em-dash character, but the scanner.Scanner barfs on that for some
// reason, producing error glyphs in the output.  It also parses it twice, which is super-annoying.  For
// this reason I've replaced it with a double-hyphen sequence, like many ASCII-limited people before me.
const testStr = `Go is a new language. Although it borrows ideas from existing languages, it has unusual properties that make effective Go programs different in character from programs written in its relatives. A straightforward translation of a C++ or Java program into Go is unlikely to produce a satisfactory result--Java programs are written in Java, not Go. On the other hand, thinking about the problem from a Go perspective could produce a successful but quite different program. In other words, to write Go well, it's important to understand its properties and idioms. It's also important to know the established conventions for programming in Go, such as naming, formatting, program construction, and so on, so that programs you write will be easy for other Go programmers to understand.

This document gives tips for writing clear, idiomatic Go code. It augments the language specification and the tutorial, both of which you should read first.

Examples
The Go package sources are intended to serve not only as the core library but also as examples of how to use the language. If you have a question about how to approach a problem or how something might be implemented, they can provide answers, ideas and background.

Formatting
Formatting issues are the most contentious but the least consequential. People can adapt to different formatting styles but it's better if they don't have to, and less time is devoted to the topic if everyone adheres to the same style. The problem is how to approach this Utopia without a long prescriptive style guide.

With Go we take an unusual approach and let the machine take care of most formatting issues. A program, gofmt, reads a Go program and emits the source in a standard style of indentation and vertical alignment, retaining and if necessary reformatting comments. If you want to know how to handle some new layout situation, run gofmt; if the answer doesn't seem right, fix the program (or file a bug), don't work around it.

As an example, there's no need to spend time lining up the comments on the fields of a structure. Gofmt will do that for you.`

const hyphStr = `Go is a new lan-guage. Although it bor-rows ideas from ex-ist-ing lan-guages, it has un-usu-al prop-er-ties that make ef-fec-tive Go pro-grams dif-fer-ent in char-ac-ter from pro-grams writ-ten in its rel-a-tives. A straight-for-ward trans-la-tion of a C++ or Ja-va pro-gram in-to Go is un-like-ly to pro-duce a sat-is-fac-to-ry re-sult--Ja-va pro-grams are writ-ten in Ja-va, not Go. On the oth-er hand, think-ing about the prob-lem from a Go per-spec-tive could pro-duce a suc-cess-ful but quite dif-fer-ent pro-gram. In oth-er words, to write Go well, it's im-por-tant to un-der-stand its prop-er-ties and id-ioms. It's al-so im-por-tant to know the es-tab-lished con-ven-tions for pro-gram-ming in Go, such as nam-ing, for-mat-ting, pro-gram con-struc-tion, and so on, so that pro-grams you write will be easy for oth-er Go pro-gram-mers to un-der-stand.

This doc-u-ment gives tips for writ-ing clear, id-iomat-ic Go code. It aug-ments the lan-guage spec-i-fi-ca-tion and the tu-to-r-i-al, both of which you should read first.

Ex-am-ples
The Go pack-age sources are in-tend-ed to serve not on-ly as the core li-brary but al-so as ex-am-ples of how to use the lan-guage. If you have a ques-tion about how to ap-proach a prob-lem or how some-thing might be im-ple-ment-ed, they can pro-vide an-swers, ideas and back-ground.

For-mat-ting
For-mat-ting is-sues are the most con-tentious but the least con-se-quen-tial. Peo-ple can adapt to dif-fer-ent for-mat-ting styles but it's bet-ter if they don't have to, and less time is de-vot-ed to the top-ic if every-one ad-heres to the same style. The prob-lem is how to ap-proach this Utopia with-out a long pre-scrip-tive style guide.

With Go we take an un-usu-al ap-proach and let the ma-chine take care of most for-mat-ting is-sues. A pro-gram, gofmt, reads a Go pro-gram and emits the source in a stan-dard style of in-den-ta-tion and ver-ti-cal align-ment, re-tain-ing and if nec-es-sary re-for-mat-ting com-ments. If you want to know how to han-dle some new lay-out sit-u-a-tion, run gofmt; if the an-swer doesn't seem right, fix the pro-gram (or file a bug), don't work around it.

As an ex-am-ple, there's no need to spend time lin-ing up the com-ments on the fields of a struc-ture. Gofmt will do that for you.`

func TestHyphenatorEnglish(t *testing.T) {
	h := buildHyphenator(t, "en-us")
	hyphenated := h.Hyphenate(testStr, `-`)

	if hyphenated != hyphStr {
		t.Fail()
	}
}

const testStrRU = `Швейк несколько лет тому назад, после того как медицинская комиссия признала его идиотом, ушёл с военной службы и теперь промышлял продажей собак, безобразных ублюдков, которым он сочинял фальшивые родословные.

Кроме того, он страдал ревматизмом и в настоящий момент растирал себе колени оподельдоком.

— Какого Фердинанда, пани Мюллерова? — спросил Швейк, не переставая массировать колени.

— Я знаю двух Фердинандов. Один служит у фармацевта Пруши. Как-то раз по ошибке он выпил у него бутылку жидкости для ращения волос; а ещё есть Фердинанд Кокошка, тот, что собирает собачье дерьмо. Обоих ни чуточки не жалко.`

const hyphStrRU = `Швейк несколь-ко лет то-му на-зад, по-с-ле то-го как ме-ди-цинская ко-мис-сия пр-из-на-ла его ид-ио-том, ушёл с во-ен-ной служ-бы и те-перь про-мыш-лял про-да-жей со-бак, бе-зобраз-ных ублюд-ков, ко-то-рым он со-чи-нял фаль-ши-вые ро-до-с-лов-ные.

Кро-ме то-го, он стра-дал рев-ма-тиз-мом и в насто-я-щий мо-мент ра-сти-рал се-бе ко-ле-ни опо-дель-до-ком.

— Ка-ко-го Фер-ди-нан-да, па-ни Мюл-ле-ро-ва? — спро-сил Швейк, не пе-ре-ста-вая мас-си-ро-вать ко-ле-ни.

— Я знаю двух Фер-ди-нан-дов. Один слу-жит у фар-ма-цев-та Пру-ши. Как-то раз по ошиб-ке он выпил у него бу-тыл-ку жид-ко-сти для ра-ще-ния во-лос; а ещё есть Фер-ди-нанд Ко-кош-ка, тот, что со-би-ра-ет со-ба-чье дерь-мо. Обо-их ни чу-точ-ки не жал-ко.`

func TestHyphenatorRussian(t *testing.T) {
	h := buildHyphenator(t, "ru")
	hyphenated := h.Hyphenate(testStrRU, `-`)

	if hyphenated != hyphStrRU {
		t.Fail()
	}
}

const testStrSpecial = `сегодня? –`
const hyphStrSpecial = `се­го­дня? –`

func TestHyphenatorSpecial(t *testing.T) {
	h := buildHyphenator(t, "ru")
	hyphenated := h.Hyphenate(testStrSpecial, `­`)

	t.Log(hyphenated)

	if hyphenated != hyphStrSpecial {
		t.Fail()
	}
}
