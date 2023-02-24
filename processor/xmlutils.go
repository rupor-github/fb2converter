package processor

import (
	"regexp"
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

func extractText(e *etree.Element, head bool) string {
	res := e.Text()
	for _, c := range e.ChildElements() {
		switch {
		case IsOneOf(c.Tag, []string{"p", "div"}):
			res += "\n" + extractText(c, false)
		case c.Tag != "a":
			res += extractText(c, false)
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
	return extractText(e, true)
}

func getFullTextFragment(e *etree.Element) string {
	// replace paragraph with new line
	res := strings.NewReplacer("\r", "", "<p>", "\n", "</p>", "").Replace(getXMLFragmentFromElement(e, false))
	// remove ALL xml tags
	res = regexp.MustCompile(`<[^(><)]+>`).ReplaceAllLiteralString(res, "")
	// remove empty lines
	lines := make([]string, 0, 16)
	for _, ss := range strings.Split(res, "\n") {
		if len(strings.TrimSpace(ss)) > 0 {
			lines = append(lines, ss)
		}
	}
	return strings.Join(lines, "\n")
}

func getXMLFragmentFromElement(e *etree.Element, format bool) string {
	d := etree.NewDocument()
	d.WriteSettings = etree.WriteSettings{CanonicalText: true, CanonicalAttrVal: true}
	d.SetRoot(e.Copy())
	if format {
		d.IndentTabs()
	}
	s, err := d.WriteToString()
	if err != nil {
		return err.Error()
	}
	return s
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
