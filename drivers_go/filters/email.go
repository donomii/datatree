package filters

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"sort"
	"strings"

	"datahumper/drivers_go/driverlib"
)

func listEmail(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse email headers: %v", err)
	default:
	}
	var resources []driverlib.Resource
	children := map[string][]byte{}
	names := make([]string, 0, len(message.Header))
	for name := range message.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range message.Header[name] {
			resources = appendChild(resources, children, "header: "+name, "text", []byte(value))
		}
	}
	parts, err := emailParts(textproto.MIMEHeader(message.Header), message.Body, "body", 0)
	switch {
	case err != nil:
		return nil, nil, err
	default:
		for _, part := range parts {
			resources = appendChild(resources, children, part.Name, part.Kind, part.Payload)
		}
		return resources, children, nil
	}
}

func emailParts(header textproto.MIMEHeader, body io.Reader, path string, depth int) ([]driverlib.Resource, error) {
	switch {
	case depth > 64:
		return nil, fmt.Errorf("cannot inspect MIME part %s: nesting exceeds 64 levels", path)
	default:
	}
	mediaType := "text/plain"
	parameters := map[string]string{}
	switch contentType := header.Get("Content-Type"); contentType {
	case "":
	default:
		var err error
		mediaType, parameters, err = mime.ParseMediaType(contentType)
		switch {
		case err != nil:
			return nil, fmt.Errorf("cannot parse MIME Content-Type for %s: %v", path, err)
		default:
		}
	}
	decoded, err := emailBody(body, header.Get("Content-Transfer-Encoding"))
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot decode MIME body %s: %v", path, err)
	default:
	}
	switch {
	case strings.HasPrefix(mediaType, "multipart/"):
		switch parameters["boundary"] {
		case "":
			return nil, fmt.Errorf("cannot read multipart body %s: boundary parameter is missing", path)
		default:
		}
		reader := multipart.NewReader(bytes.NewReader(decoded), parameters["boundary"])
		var resources []driverlib.Resource
		for index := 0; ; index++ {
			part, err := reader.NextRawPart()
			switch {
			case err == io.EOF:
				return resources, nil
			case err != nil:
				return nil, fmt.Errorf("cannot read MIME part %s/%d: %v", path, index, err)
			default:
			}
			children, readErr := emailParts(part.Header, part, fmt.Sprintf("%s/%d", path, index), depth+1)
			closeErr := part.Close()
			switch {
			case readErr != nil || closeErr != nil:
				return nil, fmt.Errorf("cannot read MIME part %s/%d: read=%v close=%v", path, index, readErr, closeErr)
			default:
				resources = append(resources, children...)
			}
		}
	default:
	}
	name := parameters["name"]
	switch disposition := header.Get("Content-Disposition"); disposition {
	case "":
	default:
		_, values, err := mime.ParseMediaType(disposition)
		switch {
		case err != nil:
			return nil, fmt.Errorf("cannot parse MIME attachment disposition %s: %v", path, err)
		case values["filename"] != "":
			name = values["filename"]
		default:
		}
	}
	kind := driverlib.DetectKind(decoded)
	switch mediaType {
	case "message/rfc822":
		kind = "email"
	case "text/html":
		kind = "html"
	case "text/plain":
		kind = "text"
	case "application/json":
		kind = "json"
	default:
	}
	switch name {
	case "":
		name = path + " (" + mediaType + ")"
	default:
		switch namedKind, known := driverlib.FormatKindForName(name); {
		case known:
			kind = namedKind
		default:
		}
	}
	return []driverlib.Resource{{Name: name, Kind: kind, Payload: decoded}}, nil
}

func emailBody(reader io.Reader, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "7bit", "8bit", "binary":
		return io.ReadAll(reader)
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(reader))
	case "base64":
		raw, err := io.ReadAll(reader)
		switch {
		case err != nil:
			return nil, err
		default:
			return base64.StdEncoding.Strict().DecodeString(strings.Join(strings.Fields(string(raw)), ""))
		}
	default:
		return nil, fmt.Errorf("unsupported Content-Transfer-Encoding %q", encoding)
	}
}

func listMbox(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	children := map[string][]byte{}
	var message bytes.Buffer
	started, inBody := false, false
	flush := func() error {
		switch _, err := mail.ReadMessage(bytes.NewReader(message.Bytes())); {
		case err != nil:
			return fmt.Errorf("cannot read mbox message %d: %v", len(resources)+1, err)
		default:
		}
		resources = appendChild(resources, children, fmt.Sprintf("[message %d]", len(resources)+1), "email", bytes.Clone(message.Bytes()))
		message.Reset()
		return nil
	}
	for _, line := range bytes.SplitAfter(raw, []byte{'\n'}) {
		switch {
		case bytes.HasPrefix(line, []byte("From ")):
			switch started {
			case true:
				switch err := flush(); {
				case err != nil:
					return nil, nil, err
				default:
				}
			default:
				started = true
			}
			inBody = false
		case !started && len(bytes.TrimSpace(line)) != 0:
			return nil, nil, fmt.Errorf("cannot parse mbox: expected a From envelope line before the first message")
		case started:
			switch {
			case inBody && bytes.HasPrefix(line, []byte{'>'}) && bytes.HasPrefix(bytes.TrimLeft(line, ">"), []byte("From ")):
				message.Write(line[1:])
			case len(bytes.TrimSpace(line)) == 0:
				inBody = true
				message.Write(line)
			default:
				message.Write(line)
			}
		default:
		}
	}
	switch started {
	case true:
		switch err := flush(); {
		case err != nil:
			return nil, nil, err
		default:
		}
	default:
	}
	return resources, children, nil
}
