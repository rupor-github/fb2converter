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

func getTextFragment(e *etree.Element) string {
	res := e.Text()
	for _, c := range e.ChildElements() {
		if IsOneOf(c.Tag, []string{"p", "div"}) {
			res += "\n" + getTextFragment(c)
		} else {
			res += getTextFragment(c)
		}
	}
	res += e.Tail()
	return strings.TrimSpace(res)
}

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
