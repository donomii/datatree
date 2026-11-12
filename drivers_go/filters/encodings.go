package filters

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"datahumper/drivers_go/driverlib"
)

func decodeBase64(raw []byte) ([]byte, error) {
	clean := bytes.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return -1
		default:
			return r
		}
	}, raw)
	encodings := []*base64.Encoding{base64.StdEncoding.Strict(), base64.RawStdEncoding.Strict(), base64.URLEncoding.Strict(), base64.RawURLEncoding.Strict()}
	var err error
	for _, encoding := range encodings {
		var result []byte
		result, err = encoding.DecodeString(string(clean))
		switch {
		case err == nil:
			return result, nil
		default:
		}
	}
	return nil, fmt.Errorf("cannot decode Base64: expected standard or URL-safe encoding: %v", err)
}

func listBase64(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	data, err := decodeBase64(raw)
	switch {
	case err != nil:
		return nil, nil, err
	default:
		return single("[decoded Base64]", driverlib.DetectKind(data), data)
	}
}

func listHex(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	clean := strings.Join(strings.Fields(string(raw)), "")
	data, err := hex.DecodeString(clean)
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot decode hexadecimal document: expected pairs of hex digits and optional whitespace: %v", err)
	default:
		return single("[decoded hexadecimal]", driverlib.DetectKind(data), data)
	}
}

func listURL(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for _, field := range strings.Split(string(raw), "&") {
		switch field {
		case "":
			continue
		default:
		}
		key, value, _ := strings.Cut(field, "=")
		name, keyErr := url.QueryUnescape(key)
		data, valueErr := url.QueryUnescape(value)
		switch {
		case keyErr != nil || valueErr != nil:
			return nil, nil, fmt.Errorf("cannot decode URL form field %q: key=%v value=%v", key, keyErr, valueErr)
		default:
			resources = appendChild(resources, children, name, driverlib.DetectKind([]byte(data)), []byte(data))
		}
	}
	return resources, children, nil
}

func listJWT(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	parts := strings.Split(strings.TrimSpace(string(raw)), ".")
	switch len(parts) {
	case 3:
	default:
		return nil, nil, fmt.Errorf("cannot inspect JWT: expected three compact JWS segments, got %d; encrypted JWE is not decoded", len(parts))
	}
	resources := []driverlib.Resource{driverlib.KV("Signature verification", "not performed; decoded claims are untrusted")}
	children := map[string][]byte{}
	for index, name := range []string{"header.json", "payload.json", "signature"} {
		data, err := base64.RawURLEncoding.Strict().DecodeString(parts[index])
		switch {
		case err != nil:
			return nil, nil, fmt.Errorf("cannot decode JWT %s segment: %v", name, err)
		case index < 2 && (!json.Valid(data) || !bytes.HasPrefix(bytes.TrimSpace(data), []byte{'{'})):
			return nil, nil, fmt.Errorf("cannot inspect JWT %s: expected a valid JSON object", name)
		default:
			resources = appendChild(resources, children, name, driverlib.DetectKind(data), data)
		}
	}
	return resources, children, nil
}

func listPEM(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	children := map[string][]byte{}
	rest := bytes.TrimSpace(raw)
	for len(rest) != 0 {
		line, _, found := bytes.Cut(rest, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		switch {
		case !found || !bytes.HasPrefix(line, []byte("-----BEGIN ")) || !bytes.HasSuffix(line, []byte("-----")):
			return nil, nil, fmt.Errorf("cannot decode PEM: expected a BEGIN line at the start of each block")
		default:
		}
		typeName := string(line[len("-----BEGIN ") : len(line)-5])
		marker := []byte("-----END " + typeName + "-----")
		end := bytes.Index(rest, marker)
		switch end {
		case -1:
			return nil, nil, fmt.Errorf("cannot decode PEM block %q: matching END line is missing", typeName)
		default:
			end += len(marker)
		}
		block, next := pem.Decode(rest[:end])
		switch {
		case block == nil || block.Type != typeName || len(bytes.TrimSpace(next)) != 0:
			return nil, nil, fmt.Errorf("cannot decode PEM block %d: expected a complete BEGIN/END block with valid Base64", len(resources))
		default:
			kind := "asn1"
			switch {
			case block.Type == "OPENSSH PRIVATE KEY" || block.Headers["Proc-Type"] != "":
				kind = "raw"
			default:
			}
			resources = appendChild(resources, children, block.Type, kind, block.Bytes)
			rest = bytes.TrimSpace(rest[end:])
		}
	}
	switch len(resources) {
	case 0:
		return nil, nil, fmt.Errorf("cannot inspect PEM: no encoded blocks found")
	default:
		return resources, children, nil
	}
}
