package processor

import (
	"strings"

	"fb2converter/etree"
)

var attr = etree.NewAttr

// getAttrValue returns value  of requested attribute or empty string.
func getAttrValue(e *etree.Element, key string) string {
	a := e.SelectAttr(key)
	if a == nil {
		return ""
	}
	return a.Value
}

func extractText(e *etree.Element, head, skipLinks bool) string {
	res := e.Text()
	for _, c := range e.ChildElements() {
		switch {
		case IsOneOf(c.Tag, []string{"p", "div", "v", "stanza"}):
			res += "\n" + extractText(c, false, skipLinks)
		case c.Tag != "a" || !skipLinks:
			res += extractText(c, false, skipLinks)
		default:
			res += c.Tail()
		}
	}
	res += e.Tail()
	if !head {
		return res
	}
	return strings.TrimSpace(res)
}

func getTextFragment(e *etree.Element) string {
	return extractText(e, true, true)
}

func getFullTextFragment(e *etree.Element) string {
	return extractText(e, true, false)
}

//lint:ignore U1000 keep getXMLFragment()
func getXMLFragment(d *etree.Document) string {
	d.IndentTabs()
	s, err := d.WriteToString()
	if err != nil {
		return err.Error()
	}
	return s
}

func getXMLFragmentFromElement(e *etree.Element) string {
	d := etree.NewDocument()
	d.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}
	d.SetRoot(e.Copy())
	d.IndentTabs()
	s, err := d.WriteToString()
	if err != nil {
		return err.Error()
	}
	return s
}
