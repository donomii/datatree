package filters

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"datahumper/drivers_go/driverlib"
)

func listINI(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case !utf8.Valid(raw):
		return nil, nil, fmt.Errorf("cannot parse INI: expected UTF-8 text")
	default:
	}
	var resources []driverlib.Resource
	children := map[string][]byte{}
	section := ""
	for index, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(line, "["):
			switch {
			case !strings.HasSuffix(line, "]") || len(line) < 3:
				return nil, nil, fmt.Errorf("cannot parse INI section on line %d: %q", index+1, line)
			default:
				section = strings.TrimSpace(line[1:len(line)-1]) + "."
			}
		default:
			position := strings.IndexAny(line, "=:")
			switch {
			case position <= 0:
				return nil, nil, fmt.Errorf("cannot parse INI line %d: expected key=value or key:value, got %q", index+1, line)
			default:
				name := strings.TrimSpace(line[:position])
				value := strings.TrimSpace(line[position+1:])
				resources = appendChild(resources, children, section+name, "text", []byte(value))
			}
		}
	}
	return resources, children, nil
}

func listDotenv(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case !utf8.Valid(raw):
		return nil, nil, fmt.Errorf("cannot parse dotenv: expected UTF-8 text")
	default:
	}
	var resources []driverlib.Resource
	children := map[string][]byte{}
	lines := strings.Split(string(raw), "\n")
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
			continue
		default:
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, found := strings.Cut(line, "=")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		switch {
		case !found || !envName(name):
			return nil, nil, fmt.Errorf("cannot parse dotenv line %d: expected a variable assignment, got %q", index+1, line)
		default:
		}
		decoded, used, err := dotenvValue(value, lines[index+1:])
		switch {
		case err != nil:
			return nil, nil, fmt.Errorf("cannot parse dotenv variable %s on line %d: %v", name, index+1, err)
		default:
			index += used
			resources = appendChild(resources, children, name, "text", []byte(decoded))
		}
	}
	return resources, children, nil
}

func envName(name string) bool {
	for index, r := range name {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || index > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return name != ""
}

func dotenvValue(value string, following []string) (string, int, error) {
	switch {
	case len(value) == 0:
		return "", 0, nil
	case value[0] != '\'' && value[0] != '"':
		for index, r := range value {
			switch {
			case r == '#' && index > 0 && (value[index-1] == ' ' || value[index-1] == '\t'):
				return strings.TrimSpace(value[:index]), 0, nil
			default:
			}
		}
		return value, 0, nil
	default:
	}
	quote := value[0]
	var out strings.Builder
	used := 0
	for index := 1; ; index++ {
		switch {
		case index >= len(value):
			switch {
			case used >= len(following):
				return "", used, fmt.Errorf("unterminated quoted value")
			default:
				value += "\n" + strings.TrimSuffix(following[used], "\r")
				used++
			}
		default:
		}
		switch {
		case value[index] == quote:
			tail := strings.TrimSpace(value[index+1:])
			switch {
			case tail == "" || strings.HasPrefix(tail, "#"):
				return out.String(), used, nil
			default:
				return "", used, fmt.Errorf("unexpected text after closing quote: %q", tail)
			}
		case quote == '"' && value[index] == '\\' && index+1 < len(value):
			index++
			switch value[index] {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '"', '\\':
				out.WriteByte(value[index])
			default:
				out.WriteByte('\\')
				out.WriteByte(value[index])
			}
		default:
			out.WriteByte(value[index])
		}
	}
}

func listProperties(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	// java.util.Properties.load(InputStream) uses ISO-8859-1, not UTF-8.
	runes := make([]rune, len(raw))
	for index, value := range raw {
		runes[index] = rune(value)
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(runes), "\r\n", "\n"), "\r", "\n"), "\n")
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for index := 0; index < len(lines); index++ {
		line := strings.TrimLeft(lines[index], " \t\f")
		switch {
		case line == "" || line[0] == '#' || line[0] == '!':
			continue
		default:
		}
		for trailingSlashes(line)%2 == 1 {
			index++
			switch {
			case index == len(lines):
				return nil, nil, fmt.Errorf("cannot parse properties: unfinished line continuation")
			default:
				line = line[:len(line)-1] + strings.TrimLeft(lines[index], " \t\f")
			}
		}
		keyEnd := len(line)
		for position := 0; position < len(line); position++ {
			switch {
			case line[position] == '\\':
				position++
			case strings.ContainsRune("=: \t\f", rune(line[position])):
				keyEnd = position
				position = len(line)
			default:
			}
		}
		value := strings.TrimLeft(line[keyEnd:], " \t\f")
		switch {
		case strings.HasPrefix(value, "=") || strings.HasPrefix(value, ":"):
			value = strings.TrimLeft(value[1:], " \t\f")
		default:
		}
		key, keyErr := propertyUnescape(line[:keyEnd])
		decoded, valueErr := propertyUnescape(value)
		switch {
		case keyErr != nil || valueErr != nil:
			return nil, nil, fmt.Errorf("cannot decode properties line %d: key=%v value=%v", index+1, keyErr, valueErr)
		default:
			resources = appendChild(resources, children, key, "text", []byte(decoded))
		}
	}
	return resources, children, nil
}

func trailingSlashes(value string) int {
	count := 0
	for index := len(value) - 1; index >= 0 && value[index] == '\\'; index-- {
		count++
	}
	return count
}

func propertyUnescape(value string) (string, error) {
	var out strings.Builder
	for len(value) != 0 {
		r, width := utf8.DecodeRuneInString(value)
		value = value[width:]
		switch {
		case r != '\\':
			out.WriteRune(r)
			continue
		case len(value) == 0:
			return "", fmt.Errorf("unfinished escape")
		default:
		}
		r, width = utf8.DecodeRuneInString(value)
		value = value[width:]
		switch r {
		case 't':
			out.WriteByte('\t')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			switch {
			case len(value) < 4:
				return "", fmt.Errorf("short Unicode escape")
			default:
			}
			code, err := strconv.ParseUint(value[:4], 16, 16)
			switch {
			case err != nil:
				return "", err
			default:
				value = value[4:]
			}
			decoded := rune(code)
			switch {
			case utf16.IsSurrogate(decoded):
				switch {
				case len(value) < 6 || !strings.HasPrefix(value, `\u`):
					return "", fmt.Errorf("unpaired Unicode surrogate")
				default:
				}
				low, err := strconv.ParseUint(value[2:6], 16, 16)
				decoded = utf16.DecodeRune(decoded, rune(low))
				switch {
				case err != nil || decoded == utf8.RuneError:
					return "", fmt.Errorf("invalid Unicode surrogate pair")
				default:
					value = value[6:]
				}
			default:
			}
			out.WriteRune(decoded)
		default:
			out.WriteRune(r)
		}
	}
	return out.String(), nil
}
