package hyphenator

import (
	// "fmt"
	// "io"
	// "os"
	"testing"
	// "text/scanner"
	// "unicode/utf8"
)

func checkValues(trie *Trie, s string, v []int, t *testing.T) {
	value, ok := trie.GetValue(s)
	values := value.([]int)
	if !ok {
		t.Fatalf("No value returned for string '%s'", s)
	}

	if len(values) != len(v) {
		t.Fatalf("Length mismatch: Values for '%s' should be %v, but got %v", s, v, values)
	}
	for i := 0; i < len(values); i++ {
		if values[i] != v[i] {
			t.Fatalf("Content mismatch: Values for '%s' should be %v, but got %v", s, v, values)
		}
	}
}

func TestTrie(t *testing.T) {
	trie := NewTrie()

	trie.AddString("hello, world!")
	trie.AddString("hello, there!")
	trie.AddString("this is a sentence.")

	if !trie.Contains("hello, world!") {
		t.Error("trie should contain 'hello, world!'")
	}
	if !trie.Contains("hello, there!") {
		t.Error("trie should contain 'hello, there!'")
	}
	if !trie.Contains("this is a sentence.") {
		t.Error("trie should contain 'this is a sentence.'")
	}
	if trie.Contains("hello, Wisconsin!") {
		t.Error("trie should NOT contain 'hello, Wisconsin!'")
	}

	expectedSize := len("hello, ") + len("world!") + len("there!") + len("this is a sentence.")
	if trie.Size() != expectedSize {
		t.Errorf("trie should contain %d nodes", expectedSize)
	}

	// insert an existing string-- should be no change
	trie.AddString("hello, world!")
	if trie.Size() != expectedSize {
		t.Errorf("trie should still contain only %d nodes after re-adding an existing member string", expectedSize)
	}

	// three strings in total
	if len(trie.Members()) != 3 {
		t.Error("trie should contain exactly three member strings")
	}

	// remove a string-- should reduce the size by the number of unique characters in that string
	trie.Remove("hello, world!")
	if trie.Contains("hello, world!") {
		t.Error("trie should no longer contain the string 'hello, world!'")
	}

	expectedSize -= len("world!")
	if trie.Size() != expectedSize {
		t.Errorf("trie should contain %d nodes after removing 'hello, world!'", expectedSize)
	}
}

func TestMultiFind(t *testing.T) {

	trie := NewTrie()

	// these are part of the matches for the word 'hyphenation'
	trie.AddString(`hyph`)
	trie.AddString(`hen`)
	trie.AddString(`hena`)
	trie.AddString(`henat`)

	expected := []string{}
	expected = append(expected, `hyph`)
	found := trie.AllSubstrings(`hyphenation`)
	if len(found) != len(expected) {
		t.Errorf("expected %v but found %v", expected, found)
	}

	expected = []string{`hen`, `hena`, `henat`}

	found = trie.AllSubstrings(`henation`)
	if len(found) != len(expected) {
		t.Errorf("expected %v but found %v", expected, found)
	}
}

///////////////////////////////////////////////////////////////
// Trie tests

func TestTrieValues(t *testing.T) {
	trie := NewTrie()

	str := "hyphenation"
	hyp := []int{0, 3, 0, 0, 2, 5, 4, 2, 0, 2, 0}

	hyphStr := "hy3phe2n5a4t2io2n"

	// test addition using separate string and vector
	trie.AddValue(str, hyp)
	if !trie.Contains(str) {
		t.Error("value trie should contain the word 'hyphenation'")
	}

	if trie.Size() != len(str) {
		t.Errorf("value trie should have %d nodes (the number of characters in 'hyphenation')", len(str))
	}

	if len(trie.Members()) != 1 {
		t.Error("value trie should have only one member string")
	}

	trie.Remove(str)
	if trie.Contains(str) {
		t.Errorf("value trie should no longer contain the word '%s'", str)
	}
	if trie.Size() != 0 {
		t.Error("value trie should have a node count of zero")
	}

	// test with an interspersed string of the form TeX's patterns use
	trie.AddPatternString(hyphStr)
	if !trie.Contains(str) {
		t.Errorf("value trie should now contain the word '%s'", str)
	}
	if trie.Size() != len(str) {
		t.Errorf("value trie should consist of %d nodes, instead has %d", len(str), trie.Size())
	}
	if len(trie.Members()) != 1 {
		t.Error("value trie should have only one member string")
	}

	mem := trie.Members()
	if mem[0] != str {
		t.Errorf("Expected first member string to be '%s', got '%s'", str, mem[0])
	}

	checkValues(trie, `hyphenation`, hyp, t)

	trie.Remove(`hyphenation`)
	if trie.Size() != 0 {
		t.Fail()
	}

	// test prefix values
	prefixedStr := `5emnix` // this is actually a string from the en_US TeX hyphenation trie
	purePrefixedStr := `emnix`
	values := []int{5, 0, 0, 0, 0, 0}
	trie.AddValue(purePrefixedStr, values)

	if trie.Size() != len(purePrefixedStr) {
		t.Errorf("Size of trie after adding '%s' should be %d, was %d", purePrefixedStr,
			len(purePrefixedStr), trie.Size())
	}

	checkValues(trie, `emnix`, values, t)

	trie.Remove(`emnix`)
	if trie.Size() != 0 {
		t.Fail()
	}

	trie.AddPatternString(prefixedStr)

	if trie.Size() != len(purePrefixedStr) {
		t.Errorf("Size of trie after adding '%s' should be %d, was %d", prefixedStr, len(purePrefixedStr),
			trie.Size())
	}

	checkValues(trie, `emnix`, values, t)
}

func TestMultiFindValue(t *testing.T) {
	trie := NewTrie()

	// these are part of the matches for the word 'hyphenation'
	trie.AddPatternString(`hy3ph`)
	trie.AddPatternString(`he2n`)
	trie.AddPatternString(`hena4`)
	trie.AddPatternString(`hen5at`)

	v1 := []int{0, 3, 0, 0}
	v2 := []int{0, 2, 0}
	v3 := []int{0, 0, 0, 4}
	v4 := []int{0, 0, 5, 0, 0}

	expectStr := []string{`hyph`}
	expectVal := []interface{}{v1}

	found, values := trie.AllSubstringsAndValues(`hyphenation`)
	if len(found) != len(expectStr) {
		t.Errorf("expected %v but found %v", expectStr, found)
	}
	if len(values) != len(expectVal) {
		t.Errorf("Length mismatch: expected %v but found %v", expectVal, values)
	}
	for i := 0; i < len(found); i++ {
		if found[i] != expectStr[i] {
			t.Errorf("Strings content mismatch: expected %v but found %v", expectStr, found)
			break
		}
	}
	for i := 0; i < len(values); i++ {
		ev := expectVal[i].([]int)
		fv := values[i].([]int)
		if len(ev) != len(fv) {
			t.Errorf("Value length mismatch: expected %v but found %v", ev, fv)
			break
		}
		for i := 0; i < len(ev); i++ {
			if ev[i] != fv[i] {
				t.Errorf("Value mismatch: expected %v but found %v", ev, fv)
				break
			}
		}
	}

	expectStr = []string{`hen`, `hena`, `henat`}
	expectVal = []interface{}{v2, v3, v4}

	found, values = trie.AllSubstringsAndValues(`henation`)
	if len(found) != len(expectStr) {
		t.Errorf("expected %v but found %v", expectStr, found)
	}
	if len(values) != len(expectVal) {
		t.Errorf("Length mismatch: expected %v but found %v", expectVal, values)
	}
	for i := 0; i < len(found); i++ {
		if found[i] != expectStr[i] {
			t.Errorf("Strings content mismatch: expected %v but found %v", expectStr, found)
			break
		}
	}
	for i := 0; i < len(values); i++ {
		ev := expectVal[i].([]int)
		fv := values[i].([]int)
		if len(ev) != len(fv) {
			t.Errorf("Value length mismatch: expected %v but found %v", ev, fv)
			break
		}
		for i := 0; i < len(ev); i++ {
			if ev[i] != fv[i] {
				t.Errorf("Value mismatch: expected %v but found %v", ev, fv)
				break
			}
		}
	}
}
