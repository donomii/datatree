package filters

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"datahumper/drivers_go/driverlib"
	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

func listTOML(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	// Validate document-wide rules (duplicate keys and table definitions) first.
	var validation struct{}
	switch err := toml.Unmarshal(raw, &validation); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse TOML document: %v", err)
	default:
	}
	var parser unstable.Parser
	parser.Reset(raw)
	var resources []driverlib.Resource
	children := map[string][]byte{}
	section := ""
	arrays := map[string]int{}
	for parser.NextExpression() {
		expression := parser.Expression()
		name := tomlKey(expression)
		switch expression.Kind {
		case unstable.Table:
			section = name + "."
			resources = append(resources, driverlib.KV("[table]", name))
		case unstable.ArrayTable:
			section = fmt.Sprintf("%s[%d].", name, arrays[name])
			resources = append(resources, driverlib.KV("[array table]", strings.TrimSuffix(section, ".")))
			arrays[name]++
		case unstable.KeyValue:
			data, err := tomlJSON(expression.Value(), 0)
			switch {
			case err != nil:
				return nil, nil, fmt.Errorf("cannot read TOML value %s%s: %v", section, name, err)
			default:
				resources = appendChild(resources, children, section+name, "json", data)
			}
		default:
		}
	}
	switch err := parser.Error(); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse TOML document: %v", err)
	default:
		return resources, children, nil
	}
}

func tomlKey(node *unstable.Node) string {
	keys := node.Key()
	var parts []string
	for keys.Next() {
		parts = append(parts, string(keys.Node().Data))
	}
	return strings.Join(parts, ".")
}

func tomlJSON(node *unstable.Node, depth int) ([]byte, error) {
	switch {
	case depth > 128:
		return nil, fmt.Errorf("value nesting exceeds 128 levels")
	default:
	}
	value := string(node.Data)
	clean := strings.ReplaceAll(value, "_", "")
	switch node.Kind {
	case unstable.String:
		return json.Marshal(value)
	case unstable.Bool:
		return []byte(value), nil
	case unstable.Integer:
		base := 10
		switch {
		case strings.HasPrefix(clean, "0x") || strings.HasPrefix(clean, "0o") || strings.HasPrefix(clean, "0b"):
			base = 0
		default:
		}
		number, err := strconv.ParseInt(clean, base, 64)
		switch {
		case err != nil:
			return nil, err
		default:
			return []byte(strconv.FormatInt(number, 10)), nil
		}
	case unstable.Float:
		number, err := strconv.ParseFloat(clean, 64)
		switch {
		case clean == "nan" || clean == "+nan" || clean == "-nan" || clean == "inf" || clean == "+inf" || clean == "-inf":
			return json.Marshal(value)
		case err != nil || math.IsInf(number, 0) || math.IsNaN(number):
			return nil, fmt.Errorf("invalid finite float %q: %v", value, err)
		default:
			return []byte(strconv.FormatFloat(number, 'g', -1, 64)), nil
		}
	case unstable.LocalDate, unstable.LocalTime, unstable.LocalDateTime, unstable.DateTime:
		layout := map[unstable.Kind]string{unstable.LocalDate: "2006-01-02", unstable.LocalTime: "15:04:05.999999999", unstable.LocalDateTime: "2006-01-02T15:04:05.999999999", unstable.DateTime: time.RFC3339Nano}[node.Kind]
		normalized := strings.ToUpper(value)
		switch {
		case len(normalized) > 10 && normalized[10] == ' ':
			normalized = normalized[:10] + "T" + normalized[11:]
		default:
		}
		switch _, err := time.Parse(layout, normalized); {
		case err != nil:
			return nil, fmt.Errorf("invalid date/time %q: %v", value, err)
		default:
			return json.Marshal(value)
		}
	case unstable.Array:
		var values []json.RawMessage
		iterator := node.Children()
		for iterator.Next() {
			data, err := tomlJSON(iterator.Node(), depth+1)
			switch {
			case err != nil:
				return nil, err
			default:
				values = append(values, data)
			}
		}
		switch values {
		case nil:
			return []byte("[]"), nil
		default:
			return json.Marshal(values)
		}
	case unstable.InlineTable:
		values := map[string]json.RawMessage{}
		iterator := node.Children()
		for iterator.Next() {
			entry := iterator.Node()
			data, err := tomlJSON(entry.Value(), depth+1)
			switch {
			case err != nil:
				return nil, err
			default:
				keys := entry.Key()
				var parts []string
				for keys.Next() {
					parts = append(parts, string(keys.Node().Data))
				}
				switch err := insertTOMLValue(values, parts, data); {
				case err != nil:
					return nil, err
				default:
				}
			}
		}
		return json.Marshal(values)
	default:
		return nil, fmt.Errorf("unsupported TOML node %s", node.Kind)
	}
}

func insertTOMLValue(values map[string]json.RawMessage, path []string, data json.RawMessage) error {
	switch len(path) {
	case 1:
		switch _, exists := values[path[0]]; {
		case exists:
			return fmt.Errorf("duplicate TOML inline key %q", path[0])
		default:
			values[path[0]] = data
			return nil
		}
	default:
		nested := map[string]json.RawMessage{}
		switch previous, exists := values[path[0]]; {
		case exists:
			switch err := json.Unmarshal(previous, &nested); {
			case err != nil:
				return fmt.Errorf("TOML dotted key conflicts with scalar %q: %v", path[0], err)
			default:
			}
		default:
		}
		switch err := insertTOMLValue(nested, path[1:], data); {
		case err != nil:
			return err
		default:
		}
		encoded, err := json.Marshal(nested)
		switch {
		case err != nil:
			return err
		default:
			values[path[0]] = encoded
			return nil
		}
	}
}
