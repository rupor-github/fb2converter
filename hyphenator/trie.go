package hyphenator

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// A Trie uses runes rather than characters for indexing, therefore its child key values are integers.
type Trie struct {
	leaf     bool           // whether the node is a leaf (the end of an input string).
	value    interface{}    // the value associated with the string up to this leaf node.
	children map[rune]*Trie // a map of sub-tries for each child rune value.
}

// NewTrie creates and returns a new Trie instance.
func NewTrie() *Trie {
	t := new(Trie)
	t.leaf = false
	t.value = nil
	t.children = make(map[rune]*Trie)
	return t
}

// Internal function: adds items to the trie, reading runes from a strings.Reader.  It returns
// the leaf node at which the addition ends.
func (p *Trie) addRunes(r *strings.Reader) *Trie {
	sym, _, err := r.ReadRune()
	if err != nil {
		p.leaf = true
		return p
	}

	n := p.children[sym]
	if n == nil {
		n = NewTrie()
		p.children[sym] = n
	}

	// recurse to store sub-runes below the new node
	return n.addRunes(r)
}

// AddString adds a string to the trie. If the string is already present, no additional storage happens. Yay!
func (p *Trie) AddString(s string) {
	if len(s) == 0 {
		return
	}

	// append the runes to the trie -- we're ignoring the value in this invocation
	p.addRunes(strings.NewReader(s))
}

// AddValue adds a string to the trie, with an associated value.  If the string is already present, only
// the value is updated.
func (p *Trie) AddValue(s string, v interface{}) {
	if len(s) == 0 {
		return
	}

	// append the runes to the trie
	leaf := p.addRunes(strings.NewReader(s))
	leaf.value = v
}

// Internal string removal function. Returns true if this node is empty following the removal.
func (p *Trie) removeRunes(r *strings.Reader) bool {
	sym, _, err := r.ReadRune()
	if err != nil {
		// remove value, remove leaf flag
		p.value = nil
		p.leaf = false
		return len(p.children) == 0
	}

	child, ok := p.children[sym]
	if ok && child.removeRunes(r) {
		// the child is now empty following the removal, so prune it
		delete(p.children, sym)
	}

	return len(p.children) == 0
}

// Remove a string from the trie. Returns true if the Trie is now empty.
func (p *Trie) Remove(s string) bool {
	if len(s) == 0 {
		return len(p.children) == 0
	}

	// remove the runes, returning the final result
	return p.removeRunes(strings.NewReader(s))
}

// Internal string inclusion function.
func (p *Trie) includes(r *strings.Reader) *Trie {
	rune, _, err := r.ReadRune()
	if err != nil {
		if p.leaf {
			return p
		}
		return nil
	}

	child, ok := p.children[rune]
	if !ok {
		return nil // no node for this rune was in the trie
	}

	// recurse down to the next node with the remainder of the string
	return child.includes(r)
}

// Contains tests for the inclusion of a particular string in the Trie.
func (p *Trie) Contains(s string) bool {
	if len(s) == 0 {
		return false // empty strings can't be included (how could we add them?)
	}
	return p.includes(strings.NewReader(s)) != nil
}

// GetValue returns the value associated with the given string.  Double return: false if the given string was
// not present, true if the string was present.  The value could be both valid and nil.
func (p *Trie) GetValue(s string) (interface{}, bool) {
	if len(s) == 0 {
		return nil, false
	}

	leaf := p.includes(strings.NewReader(s))
	if leaf == nil {
		return nil, false
	}
	return leaf.value, true
}

// Internal output-building function used by Members()
func (p *Trie) buildMembers(prefix string) []string {

	strList := []string{}
	if p.leaf {
		strList = append(strList, prefix)
	}

	// for each child, go grab all suffixes
	for sym, child := range p.children {
		buf := make([]byte, 4)
		numChars := utf8.EncodeRune(buf, sym)
		strList = append(strList, child.buildMembers(prefix+string(buf[0:numChars]))...)
	}
	return strList
}

// Members retrieves all member strings, in order.
func (p *Trie) Members() (members []string) {
	members = p.buildMembers(``)
	sort.Strings(members)
	return
}

// Size counts all the nodes of the entire Trie, NOT including the root node.
func (p *Trie) Size() (sz int) {
	sz = len(p.children)

	for _, child := range p.children {
		sz += child.Size()
	}

	return
}

// AllSubstrings returns all anchored substrings of the given string within the Trie.
func (p *Trie) AllSubstrings(s string) []string {

	v := []string{}

	for pos, rune := range s {
		child, ok := p.children[rune]
		if !ok {
			// return whatever we have so far
			break
		}

		// if this is a leaf node, add the string so far to the output vector
		if child.leaf {
			v = append(v, s[0:pos])
		}
		p = child
	}
	return v
}

// AllSubstringsAndValues returns all anchored substrings of the given string within the Trie, with a matching set of
// their associated values.
func (p *Trie) AllSubstringsAndValues(s string) ([]string, []interface{}) {

	sv := []string{}
	vv := []interface{}{}

	for pos, rune := range s {
		child, ok := p.children[rune]
		if !ok {
			// return whatever we have so far
			break
		}

		// if this is a leaf node, add the string so far and its value
		if child.leaf {
			sv = append(sv, s[0:pos+utf8.RuneLen(rune)])
			vv = append(vv, child.value)
		}
		p = child
	}
	return sv, vv
}
