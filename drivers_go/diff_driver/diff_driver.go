package diffdriver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"datahumper/drivers_go/driverlib"
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)$`)

func Run() {
	driverlib.Run("Diff driver", listDiffLines)
}

func listDiffLines(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	lines := strings.SplitAfter(string(raw), "\n")
	children := map[string][]byte{}
	resources := make([]driverlib.Resource, 0, len(lines))
	oldLine := int64(0)
	newLine := int64(0)
	inHunk := false
	for _, original := range lines {
		line := strings.TrimSuffix(strings.TrimSuffix(original, "\n"), "\r")
		switch {
		case line == "" && !inHunk:
			continue
		case hunkHeaderPattern.MatchString(line):
			matches := hunkHeaderPattern.FindStringSubmatch(line)
			parsedOldLine, oldErr := parseLineNumber(matches[1], line)
			switch {
			case oldErr != nil:
				return nil, nil, oldErr
			default:
			}
			parsedNewLine, newErr := parseLineNumber(matches[2], line)
			switch {
			case newErr != nil:
				return nil, nil, newErr
			default:
			}
			oldLine = parsedOldLine
			newLine = parsedNewLine
			inHunk = true
			resources = append(resources, diffResource("diff-hunk", line))
		case !inHunk:
			resources = append(resources, diffResource("diff-meta", line))
		case strings.HasPrefix(line, "+"):
			resources = append(resources, numberedDiffResource("+", "diff-add", 0, newLine, strings.TrimPrefix(line, "+")))
			newLine++
		case strings.HasPrefix(line, "-"):
			resources = append(resources, numberedDiffResource("-", "diff-delete", oldLine, 0, strings.TrimPrefix(line, "-")))
			oldLine++
		case strings.HasPrefix(line, " "):
			resources = append(resources, numberedDiffResource(" ", "diff-context", oldLine, newLine, strings.TrimPrefix(line, " ")))
			oldLine++
			newLine++
		case strings.HasPrefix(line, "\\"):
			resources = append(resources, diffResource("diff-meta", line))
		default:
			inHunk = false
			resources = append(resources, diffResource("diff-meta", line))
		}
		// Preserve the actual input line, independently of the decorated label.
		key := fmt.Sprintf("line-%d", len(resources)-1)
		resources[len(resources)-1].Key = key
		resources[len(resources)-1].Size = int64(len(original))
		children[key] = []byte(original)
	}
	return resources, children, nil
}

func parseLineNumber(raw string, header string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	switch {
	case err != nil:
		return 0, fmt.Errorf("cannot parse unified diff hunk header %q: expected line number %q to fit an integer, got: %v", header, raw, err)
	default:
		return value, nil
	}
}

func numberedDiffResource(marker string, kind string, oldLine int64, newLine int64, text string) driverlib.Resource {
	oldLabel := ""
	newLabel := ""
	switch {
	case oldLine > 0:
		oldLabel = strconv.FormatInt(oldLine, 10)
	default:
	}
	switch {
	case newLine > 0:
		newLabel = strconv.FormatInt(newLine, 10)
	default:
	}
	name := fmt.Sprintf("%s %6s %6s | %s", marker, oldLabel, newLabel, text)
	return diffResource(kind, name)
}

func diffResource(kind string, name string) driverlib.Resource {
	return driverlib.Resource{Name: name, Kind: kind, Size: int64(len(name))}
}
