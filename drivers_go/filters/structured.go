package filters

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"datahumper/drivers_go/driverlib"
	"go.yaml.in/yaml/v3"
)

func listNDJSON(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for index, line := range bytes.Split(raw, []byte{'\n'}) {
		switch {
		case len(bytes.TrimSpace(line)) == 0:
			continue
		case !json.Valid(line):
			return nil, nil, fmt.Errorf("cannot read JSON Lines record at line %d: expected one complete JSON value", index+1)
		default:
			resources = appendChild(resources, children, fmt.Sprintf("[line %d]", index+1), "json", line)
		}
	}
	return resources, children, nil
}

func listYAML(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var documents []*yaml.Node
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		switch {
		case err == io.EOF:
			return yamlDocuments(documents)
		case err != nil:
			return nil, nil, fmt.Errorf("cannot parse YAML document %d: %v", len(documents)+1, err)
		default:
			budget := 100000
			copy, err := copyYAML(&document, map[*yaml.Node]bool{}, &budget)
			switch {
			case err != nil:
				return nil, nil, err
			default:
				documents = append(documents, copy.Content[0])
			}
		}
	}
}

// Resolve aliases into self-contained fragments. Cycles cannot form finite bytes.
func copyYAML(node *yaml.Node, ancestors map[*yaml.Node]bool, budget *int) (*yaml.Node, error) {
	*budget--
	switch {
	case *budget < 0 || len(ancestors) > 128:
		return nil, fmt.Errorf("cannot expand YAML: exceeded 100000 nodes or 128 nesting levels")
	case ancestors[node]:
		return nil, fmt.Errorf("cannot expand cyclic YAML alias at line %d; original bytes remain available", node.Line)
	default:
	}
	ancestors[node] = true
	defer delete(ancestors, node)
	switch node.Kind {
	case yaml.AliasNode:
		return copyYAML(node.Alias, ancestors, budget)
	default:
	}
	copy := *node
	copy.Anchor, copy.Alias, copy.Content = "", nil, nil
	seen := map[string]bool{}
	for index, child := range node.Content {
		value, err := copyYAML(child, ancestors, budget)
		switch {
		case err != nil:
			return nil, err
		default:
		}
		switch {
		case node.Kind == yaml.MappingNode && index%2 == 0:
			key, err := yaml.Marshal(value)
			switch {
			case err != nil || seen[string(key)]:
				return nil, fmt.Errorf("cannot read YAML mapping at line %d: duplicate or invalid key %q: %v", child.Line, key, err)
			default:
				seen[string(key)] = true
			}
		default:
		}
		copy.Content = append(copy.Content, value)
	}
	return &copy, nil
}

func yamlDocuments(documents []*yaml.Node) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	children := map[string][]byte{}
	switch len(documents) {
	case 0:
		return resources, children, nil
	case 1:
		root := documents[0]
		switch root.Kind {
		case yaml.MappingNode:
			for index := 0; index < len(root.Content); index += 2 {
				key := root.Content[index]
				name := key.Value
				switch key.Kind {
				case yaml.ScalarNode:
				default:
					encoded, err := yaml.Marshal(key)
					switch {
					case err != nil:
						return nil, nil, err
					default:
						name = string(encoded)
					}
				}
				var err error
				resources, err = yamlChild(resources, children, name, root.Content[index+1])
				switch {
				case err != nil:
					return nil, nil, err
				default:
				}
			}
		case yaml.SequenceNode:
			for index, child := range root.Content {
				var err error
				resources, err = yamlChild(resources, children, fmt.Sprintf("[%d]", index), child)
				switch {
				case err != nil:
					return nil, nil, err
				default:
				}
			}
		default:
			var err error
			resources, err = yamlChild(resources, children, "[value]", root)
			switch {
			case err != nil:
				return nil, nil, err
			default:
			}
		}
	default:
		for index, document := range documents {
			var err error
			resources, err = yamlChild(resources, children, fmt.Sprintf("[document %d]", index+1), document)
			switch {
			case err != nil:
				return nil, nil, err
			default:
			}
		}
	}
	return resources, children, nil
}

func yamlChild(resources []driverlib.Resource, children map[string][]byte, name string, node *yaml.Node) ([]driverlib.Resource, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		data := []byte(node.Value)
		switch node.Tag {
		case "!!binary":
			var err error
			data, err = base64.StdEncoding.DecodeString(node.Value)
			switch {
			case err != nil:
				return nil, fmt.Errorf("cannot decode YAML binary value %q: %v", name, err)
			default:
				return appendChild(resources, children, name, driverlib.DetectKind(data), data), nil
			}
		default:
			return appendChild(resources, children, name, "text", data), nil
		}
	default:
		data, err := yaml.Marshal(node)
		switch {
		case err != nil:
			return nil, fmt.Errorf("cannot encode YAML child %q: %v", name, err)
		default:
			return appendChild(resources, children, name, "yaml", data), nil
		}
	}
}
