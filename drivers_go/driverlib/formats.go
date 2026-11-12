package driverlib

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// AdditionalFilters is the shared routing and user-help catalog for new filters.
func AdditionalFilters() []FilterInfo {
	return []FilterInfo{
		{"auto_driver", "auto", "Detect a format from its bytes, then open the detected document; unknown bytes remain readable.", nil, false},
		{"bundle_driver", "bundle", "Open Datatree byte bundles such as SQLite tables and rows, preserving binary cells exactly.", nil, false},
		{"jsonstring_driver", "jsonstring", "Decode a serialized JSON string to its actual UTF-8 contents before applying another filter.", nil, true},
		{"tar_driver", "tar", "Browse TAR members without extracting files or following links.", []string{".tar"}, false},
		{"gzip_driver", "gzip", "Decompress GZIP and filter the complete decompressed bytes again.", []string{".gz", ".tgz"}, false},
		{"bzip2_driver", "bzip2", "Decompress BZIP2; apply TAR or another filter to the result.", []string{".bz2", ".tbz", ".tbz2"}, false},
		{"xz_driver", "xz", "Decompress XZ or an XZ-compressed TAR archive.", []string{".xz", ".txz"}, false},
		{"zstd_driver", "zstd", "Decompress Zstandard and inspect the resulting bytes.", []string{".zst", ".zstd", ".tzst"}, false},
		{"zlib_driver", "zlib", "Decode a ZLIB-wrapped DEFLATE stream.", []string{".zlib", ".zz"}, false},
		{"deflate_driver", "deflate", "Decode a raw DEFLATE stream without a ZLIB or GZIP header.", []string{".deflate"}, false},
		{"xml_driver", "xml", "Browse XML elements, attributes and text, including SVG, feeds and XML property lists; external entities are never loaded.", []string{".xml", ".svg", ".rss", ".atom", ".xhtml", ".xsd", ".xsl", ".xslt", ".kml", ".gpx", ".plist", ".rels"}, true},
		{"yaml_driver", "yaml", "Browse YAML documents, mappings, sequences and scalar values; finite aliases become self-contained children.", []string{".yaml", ".yml"}, true},
		{"toml_driver", "toml", "Browse TOML tables and typed values, including arrays and inline tables.", []string{".toml"}, true},
		{"ndjson_driver", "ndjson", "Read each nonblank JSON Lines record as an independent JSON child; malformed records are errors.", []string{".jsonl", ".ndjson"}, true},
		{"ini_driver", "ini", "Browse INI sections and key/value entries; repeated keys retain independent bytes.", []string{".ini", ".cfg", ".conf"}, true},
		{"properties_driver", "properties", "Decode Java properties, including escaped Unicode and continued lines.", []string{".properties"}, true},
		{"dotenv_driver", "dotenv", "Read literal dotenv assignments, including quotes, without expanding variables or executing commands.", []string{".env"}, true},
		{"email_driver", "email", "Browse RFC 5322 headers and recursively decode MIME bodies and attachments.", []string{".eml", ".mime"}, true},
		{"mbox_driver", "mbox", "Split an mbox mailbox into complete messages for the email filter.", []string{".mbox"}, true},
		{"base64_driver", "base64", "Decode standard or URL-safe Base64; the decoded bytes can use any filter.", []string{".b64", ".base64"}, true},
		{"hex_driver", "hex", "Decode hexadecimal text with optional whitespace into actual bytes.", []string{".hex"}, true},
		{"urlencoded_driver", "urlencoded", "Decode URL query/form fields; preserve repeated fields as independent values.", []string{".urlencoded"}, true},
		{"jwt_driver", "jwt", "Inspect a compact JWT header, payload and signature bytes; this does not verify the signature.", []string{".jwt"}, true},
		{"pem_driver", "pem", "Decode each PEM block to its DER or other original payload bytes, without decrypting keys.", []string{".pem", ".crt", ".key", ".pub"}, true},
		{"asn1_driver", "asn1", "Browse ASN.1 DER values and constructed children, including certificates and unencrypted keys.", []string{".der", ".cer"}, false},
		{"protobuf_driver", "protobuf", "Inspect Protocol Buffers wire fields by number and wire type; length-delimited fields remain exact bytes because no schema is assumed.", []string{".pb", ".protobuf"}, false},
		{"sqlite_driver", "sqlite", "Browse SQLite tables, rows and exact BLOB cells in an isolated read-only in-memory database.", []string{".sqlite", ".sqlite3", ".db", ".db3"}, false},
		{"riff_driver", "riff", "Browse RIFF chunks in WAVE, AVI and WebP containers as original bytes.", []string{".wav", ".wave", ".avi"}, false},
		{"mp4_driver", "mp4", "Browse ISO base media boxes in MP4, MOV, M4A, AVIF and HEIF containers as original bytes.", []string{".mp4", ".mov", ".m4a", ".m4v", ".heic", ".heif", ".avif"}, false},
	}
}

func FilterCatalog() []FilterInfo {
	filters := []FilterInfo{
		{"diff_driver", "diff", "Browse unified diff hunks and their lines.", []string{".diff", ".patch"}, true},
		{"zip_driver", "archive", "Browse ZIP members, including Office, OpenDocument, EPUB and packaged application files.", []string{".zip", ".docx", ".xlsx", ".pptx", ".docm", ".xlsm", ".pptm", ".odt", ".ods", ".odp", ".epub", ".jar", ".war", ".ear", ".apk", ".aab", ".ipa", ".whl", ".nupkg", ".vsix", ".kmz", ".cbz"}, false},
		{"text_driver", "text", "Read UTF-8 text and browse extracted links; binary input remains downloadable.", []string{".txt", ".md", ".log"}, true},
		{"json_driver", "json", "Browse JSON objects, arrays and exact serialized scalar values.", []string{".json", ".geojson", ".ipynb", ".har", ".map"}, true},
		{"image_driver", "image", "Preview image bytes and inspect format and dimensions.", []string{".jpg", ".jpeg", ".png", ".gif", ".bmp"}, false},
		{"pdf_driver", "pdf", "Preview PDF bytes, version, available page count and links.", []string{".pdf"}, false},
		{"html_driver", "html", "Read HTML source, title and links without executing the document.", []string{".html", ".htm"}, true},
		{"csv_driver", "csv", "Browse comma-separated headings and string-valued rows as JSON.", []string{".csv"}, true},
		{"tsv_driver", "tsv", "Browse tab-separated headings and string-valued rows as JSON.", []string{".tsv", ".tab"}, true},
	}
	return append(filters, AdditionalFilters()...)
}

func FormatKindForName(name string) (string, bool) {
	lower := strings.ToLower(name)
	for _, format := range FilterCatalog() {
		for _, extension := range format.Extensions {
			switch {
			case strings.HasSuffix(lower, extension):
				return format.Kind, true
			default:
			}
		}
	}
	return "", false
}

// DetectKind uses unambiguous signatures before conservative textual detection.
func DetectKind(raw []byte) string {
	signatures := []struct {
		magic []byte
		kind  string
	}{
		{[]byte("PK\x03\x04"), "archive"}, {[]byte("PK\x05\x06"), "archive"},
		{[]byte{0x1f, 0x8b}, "gzip"}, {[]byte("BZh"), "bzip2"},
		{[]byte{0xfd, '7', 'z', 'X', 'Z', 0}, "xz"}, {[]byte{0x28, 0xb5, 0x2f, 0xfd}, "zstd"},
		{[]byte("SQLite format 3\x00"), "sqlite"}, {[]byte("RIFF"), "riff"},
		{[]byte("%PDF-"), "pdf"}, {[]byte("\x89PNG\r\n\x1a\n"), "image"},
		{[]byte{0xff, 0xd8, 0xff}, "image"}, {[]byte("GIF8"), "image"}, {[]byte("BM"), "image"},
	}
	for _, signature := range signatures {
		switch {
		case bytes.HasPrefix(raw, signature.magic):
			return signature.kind
		default:
		}
	}
	trimmed := bytes.TrimSpace(raw)
	switch {
	case len(raw) >= 262 && string(raw[257:262]) == "ustar":
		return "tar"
	case len(raw) >= 12 && string(raw[4:8]) == "ftyp":
		return "mp4"
	case bytes.HasPrefix(trimmed, []byte("-----BEGIN ")):
		return "pem"
	case json.Valid(trimmed):
		return "json"
	case bytes.HasPrefix(bytes.ToLower(trimmed), []byte("<!doctype html")) || bytes.HasPrefix(bytes.ToLower(trimmed), []byte("<html")):
		return "html"
	case bytes.HasPrefix(trimmed, []byte("<")):
		return "xml"
	case bytes.HasPrefix(trimmed, []byte("---\n")) || bytes.HasPrefix(trimmed, []byte("---\r\n")):
		return "yaml"
	case utf8.Valid(raw) && !bytes.ContainsRune(raw, 0):
		return "text"
	default:
		return "raw"
	}
}
