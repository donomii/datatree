package filters

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"

	"datahumper/drivers_go/driverlib"
)

type xmlElement struct {
	name     xml.Name
	attrs    []xml.Attr
	start    int64
	raw      []byte
	children []xmlPart
}

type xmlPart struct {
	name    string
	text    []byte
	element *xmlElement
}

func listXML(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var root *xmlElement
	var stack []*xmlElement
	for {
		start := decoder.InputOffset()
		token, err := decoder.Token()
		switch {
		case err == io.EOF:
			return xmlChildren(root)
		case err != nil:
			return nil, nil, fmt.Errorf("cannot parse XML at byte %d: %v", start, err)
		default:
		}
		switch value := token.(type) {
		case xml.StartElement:
			node := &xmlElement{name: value.Name, attrs: value.Attr, start: start}
			switch {
			case len(stack) > 128:
				return nil, nil, fmt.Errorf("cannot browse XML: nesting exceeds 128 levels")
			case len(stack) == 0 && root != nil:
				return nil, nil, fmt.Errorf("cannot parse XML: expected one root element, found another at byte %d", start)
			case len(stack) == 0:
				root = node
			default:
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, xmlPart{element: node})
			}
			stack = append(stack, node)
		case xml.EndElement:
			node := stack[len(stack)-1]
			node.raw = raw[node.start:decoder.InputOffset()]
			stack = stack[:len(stack)-1]
		case xml.CharData:
			switch {
			case len(stack) == 0 && len(bytes.TrimSpace(value)) != 0:
				return nil, nil, fmt.Errorf("cannot parse XML: non-whitespace outside the root element at byte %d", start)
			case len(stack) != 0:
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, xmlPart{name: "[text]", text: bytes.Clone(value)})
			default:
			}
		case xml.Comment:
			switch len(stack) {
			case 0:
			default:
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, xmlPart{name: "[comment]", text: bytes.Clone(value)})
			}
		default:
			// Processing instructions and declarations are retained in source bytes.
		}
	}
}

func xmlChildren(root *xmlElement) ([]driverlib.Resource, map[string][]byte, error) {
	switch root {
	case nil:
		return nil, nil, fmt.Errorf("cannot browse XML: expected a root element")
	default:
	}
	resources := []driverlib.Resource{driverlib.KV("element", xmlName(root.name))}
	children := map[string][]byte{}
	for _, attribute := range root.attrs {
		resources = appendChild(resources, children, "@"+xmlName(attribute.Name), "text", []byte(attribute.Value))
	}
	for _, part := range root.children {
		switch part.element {
		case nil:
			resources = appendChild(resources, children, part.name, "text", part.text)
		default:
			resources = appendChild(resources, children, xmlName(part.element.name), "xml", part.element.raw)
		}
	}
	return resources, children, nil
}

func xmlName(name xml.Name) string {
	switch name.Space {
	case "":
		return name.Local
	default:
		return "{" + name.Space + "}" + name.Local
	}
}
