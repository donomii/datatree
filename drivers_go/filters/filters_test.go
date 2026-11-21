package filters

import (
	"bytes"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"

	"datahumper/drivers_go/driverlib"
)

func childBytes(t *testing.T, resources []driverlib.Resource, children map[string][]byte, name string) []byte {
	t.Helper()
	for _, resource := range resources {
		switch resource.Name {
		case name:
			data, exists := children[resource.Key]
			switch exists {
			case true:
				return data
			default:
				t.Fatalf("%q has no byte payload", name)
			}
		default:
		}
	}
	t.Fatalf("child %q missing from %+v", name, resources)
	return nil
}

func TestStructuredAndEncodedFilters(t *testing.T) {
	fixtures := []struct{ driver, raw, name, want string }{
		{"auto_driver", `{"value":42}`, "[detected document]", `{"value":42}`},
		{"xml_driver", `<root id="A&amp;B"><child>actual &lt;text&gt;</child></root>`, "child", `<child>actual &lt;text&gt;</child>`},
		{"yaml_driver", "name: actual data\nempty: ''\n", "name", "actual data"},
		{"toml_driver", "[section]\nvalues = [1, \"two\", true]\n", "section.values", `[1,"two",true]`},
		{"toml_driver", `value = { a.b = 1, "a.b" = 2 }`, "value", `{"a":{"b":1},"a.b":2}`},
		{"ndjson_driver", "\n {\"a\":1}\r\n{\"a\":2}\n", "[line 2]", " {\"a\":1}\r"},
		{"ini_driver", "[section]\nname=actual data\nname=second\n", "section.name", "actual data"},
		{"properties_driver", "a\\ b = first\\\n  second\\u0021\nemoji=\\uD83D\\uDE03\n", "a b", "firstsecond!"},
		{"dotenv_driver", "A=\"first\\n$UNCHANGED\"\nB='$(untouched)'\n", "A", "first\n$UNCHANGED"},
		{"dotenv_driver", "A=\"line1\nline2\" # comment\n", "A", "line1\nline2"},
		{"email_driver", "From: a@example.test\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nactual=20data", "body (text/plain)", "actual data"},
		{"base64_driver", base64.StdEncoding.EncodeToString([]byte("actual\x00data")), "[decoded Base64]", "actual\x00data"},
		{"hex_driver", "61 63 74 75 61 6c 00 64 61 74 61\n", "[decoded hexadecimal]", "actual\x00data"},
		{"urlencoded_driver", "a=first+value&a=second&b=%00%FF", "a", "first value"},
		{"jsonstring_driver", `"actual\tdata"`, "[decoded string]", "actual\tdata"},
		{"asn1_driver", string([]byte{4, 3, 'a', 0, 'b'}), "[class 0 tag 4 value]", "a\x00b"},
		{"protobuf_driver", string([]byte{10, 3, 'a', 0, 'b', 16, 42}), "field 1 (wire 2)", "a\x00b"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.driver+"/"+fixture.name, func(t *testing.T) {
			parser, ok := Parser(fixture.driver)
			switch ok {
			case false:
				t.Fatal("missing filter")
			default:
			}
			resources, children, err := driverlib.WithSource(parser)([]byte(fixture.raw))
			switch {
			case err != nil:
				t.Fatal(err)
			default:
			}
			got := childBytes(t, resources, children, fixture.name)
			switch {
			case string(got) != fixture.want:
				t.Fatalf("payload %q expected %q", got, fixture.want)
			case !bytes.Equal(childBytes(t, resources, children, "[source bytes]"), []byte(fixture.raw)):
				t.Fatal("lost exact source")
			default:
			}
		})
	}
}

func TestEveryAdvertisedFilterHasAParser(t *testing.T) {
	for _, format := range driverlib.AdditionalFilters() {
		switch _, ok := Parser(format.Driver); {
		case !ok:
			t.Errorf("%s advertises an unavailable parser", format.Driver)
		default:
		}
	}
}

func TestMalformedInputsKeepSourceAndReportErrors(t *testing.T) {
	fixtures := map[string]string{
		"tar_driver": "broken", "gzip_driver": "broken", "bzip2_driver": "broken", "xz_driver": "broken", "zstd_driver": "broken", "zlib_driver": "broken", "deflate_driver": "broken",
		"xml_driver": "<root><bad></root>", "yaml_driver": "a: [unfinished", "toml_driver": "a=1\na=2", "ndjson_driver": "{}\nnot json",
		"ini_driver": "invalid entry", "properties_driver": `a=\uXYZW`, "dotenv_driver": "A='unterminated", "email_driver": "bad header\n\ntext", "mbox_driver": "missing envelope",
		"base64_driver": "!invalid!", "hex_driver": "ABC", "urlencoded_driver": "a=%ZZ", "jwt_driver": "x.y.z", "pem_driver": "-----BEGIN INVALID-----\n!\n-----END INVALID-----",
		"asn1_driver": string([]byte{0x30, 0x80}), "protobuf_driver": string([]byte{10, 100}), "sqlite_driver": "not sqlite", "riff_driver": "RIFF", "mp4_driver": "bad", "lance_driver": "bad", "bundle_driver": `{"format":"other"}`, "jsonstring_driver": "42",
	}
	for name, input := range fixtures {
		t.Run(name, func(t *testing.T) {
			parser, ok := Parser(name)
			switch ok {
			case false:
				t.Fatal("missing parser")
			default:
			}
			resources, children, err := driverlib.WithSource(parser)([]byte(input))
			switch {
			case err != nil || len(resources) != 2 || resources[0].Kind != "error":
				t.Fatalf("expected visible error and source: %+v %v", resources, err)
			case string(childBytes(t, resources, children, "[source bytes]")) != input:
				t.Fatal("malformed input lost bytes")
			default:
			}
		})
	}
}

func TestYAMLAliasesAndNestedXML(t *testing.T) {
	resources, children, err := listYAML([]byte("base: &base {value: actual}\ncopy: *base\n"))
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	copy := childBytes(t, resources, children, "copy")
	resources, children, err = listYAML(copy)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch string(childBytes(t, resources, children, "value")) {
	case "actual":
	default:
		t.Fatal("alias fragment lost value")
	}
	for _, raw := range []string{"a: &a [*a]", "a: 1\na: 2\n"} {
		_, _, err := listYAML([]byte(raw))
		switch {
		case err == nil:
			t.Fatalf("accepted invalid/cyclic YAML %s", raw)
		default:
		}
	}
	resources, children, err = listXML([]byte(`<r xmlns="urn:test"><child>actual &amp; data</child></r>`))
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	fragment := childBytes(t, resources, children, "{urn:test}child")
	resources, children, err = listXML(fragment)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch string(childBytes(t, resources, children, "[text]")) {
	case "actual & data":
	default:
		t.Fatal("XML fragment lost text")
	}
}

func TestEmailAttachmentsJWTPEMAndMbox(t *testing.T) {
	payload := []byte("PK\x03\x04actual bytes\x00")
	email := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=test\r\n\r\n--test\r\nContent-Type: application/zip\r\nContent-Disposition: attachment; filename=archive.zip\r\nContent-Transfer-Encoding: base64\r\n\r\n" + base64.StdEncoding.EncodeToString(payload) + "\r\n--test--\r\n"
	resources, children, err := listEmail([]byte(email))
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch {
	case !bytes.Equal(childBytes(t, resources, children, "archive.zip"), payload):
		t.Fatal("MIME attachment changed bytes")
	default:
	}
	jwt := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"value":42}`)) + "."
	resources, children, err = listJWT([]byte(jwt))
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch string(childBytes(t, resources, children, "payload.json")) {
	case `{"value":42}`:
	default:
		t.Fatal("JWT claims lost")
	}
	der := []byte{0x30, 3, 2, 1, 42}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	resources, children, err = listPEM(encoded)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch {
	case !bytes.Equal(childBytes(t, resources, children, "CERTIFICATE"), der):
		t.Fatal("PEM changed DER")
	default:
	}
	resources, children, err = listMbox([]byte("From sender now\nSubject: one\n\n>From body\nFrom other now\nSubject: two\n\nsecond\n"))
	switch {
	case err != nil || len(resources) != 2:
		t.Fatalf("mbox messages: %+v %v", resources, err)
	case !strings.Contains(string(childBytes(t, resources, children, "[message 1]")), "\nFrom body\n"):
		t.Fatal("mboxrd body did not unescape From")
	default:
	}
}

func FuzzByteFilters(f *testing.F) {
	for _, seed := range [][]byte{nil, []byte("---\n"), []byte("a: &a [*a]"), []byte("<root/>"), []byte{0x1f, 0x8b}, []byte{0x30, 0x80}, []byte("RIFF"), []byte("[section]\na=[1,2]\n")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		switch {
		case len(raw) > 65536:
			t.Skip()
		default:
		}
		for _, format := range driverlib.AdditionalFilters() {
			parser, _ := Parser(format.Driver)
			resources, children, err := driverlib.WithSource(parser)(raw)
			switch {
			case err != nil:
				t.Fatalf("%s failed to retain a view: %v", format.Driver, err)
			case !bytes.Equal(childBytes(t, resources, children, "[source bytes]"), raw):
				t.Fatalf("%s changed source bytes", format.Driver)
			default:
			}
		}
	})
}
