package processor

import (
	"testing"

	"github.com/rupor-github/fb2converter/etree"
)

var casesTxtFragment = []testCase{
	testCase{
		in:  `<p><emphasis>Молоток</emphasis> — небольшой ударный ручной инструмент, применяемый для забивания <emphasis>Гвоздей</emphasis>.</p>`,
		out: `Молоток — небольшой ударный ручной инструмент, применяемый для забивания Гвоздей.`,
	},
}

func TestTextFragment(t *testing.T) {

	for i, c := range casesTxtFragment {
		d := etree.NewDocument()
		if err := d.ReadFromString(c.in); err != nil {
			t.Fatal(err)
		}
		res := getTextFragment(d.Root())
		if res != c.out {
			t.Fatalf("BAD RESULT for case %d\nEXPECTED:\n[%s]\nGOT:\n[%s]", i+1, c.out, res)
		}
	}
	t.Logf("OK - %s: %d cases", t.Name(), len(cases))
}
