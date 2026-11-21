# Byte filters

Use **Filter → Apply filter** on any readable resource. Each result can be filtered again, including archive members, decoded values, attachments, database cells and S3 objects. Every filter retains the complete original input in **[source bytes]**, including invalid input. Downloads always use the resource's byte handle.

Uploads default to **AUTO**: known filename extensions choose their reader; otherwise signatures identify the bytes. Unknown files in the tree also use byte detection. Unrecognized data remains readable through the paged hexadecimal/text preview and explicit filters. No filter extracts files to disk, executes document content or fetches document links.

The selector explains each filter. Names are case insensitive for extension detection. `/api/formats` returns the same catalog used by the UI and gallery; `/api/drivers` retains its array of runnable filter names.

| Filter | Contents exposed |
| --- | --- |
| AUTO | Detected document, or unrecognized raw bytes. Recognizes ZIP, GZIP, BZIP2, XZ, Zstandard, TAR, SQLite, Lance, RIFF, MP4, PDF, common images, PEM, JSON, HTML, XML, YAML document markers and UTF-8 text. |
| ZIP / archive | Original member bytes, with independent handles for duplicate names. Includes DOCX/XLSX/PPTX, their macro-enabled variants, OpenDocument, EPUB, JAR/WAR/EAR, APK/AAB/IPA, Python wheels, NuGet, VSIX, KMZ and CBZ. Document internals are browsed as archive members, not rendered as an office application would. |
| TAR | Regular member bytes; link targets as text, without following them. Special entry types are identified. Duplicate member names remain independent. |
| GZIP | Complete decompressed stream. A stored filename chooses its kind; otherwise signatures detect it. Concatenated members are decoded in stream order. |
| BZIP2, XZ, Zstandard | Complete decompressed bytes with signature-based classification. This also opens compressed TARs. |
| ZLIB, DEFLATE | Complete decompressed bytes, with or without the ZLIB wrapper respectively. |
| JSON | Objects, arrays and serialized scalar values. Includes GeoJSON, notebooks, HAR and JSON source maps. |
| JSONSTRING | Actual contents of one serialized JSON string, useful before Base64 or another decoder. |
| JSON Lines / NDJSON | Independent records with original line bytes; blank lines are ignored and malformed records are errors. |
| CSV, TSV | Counts, headers and JSON row objects. Quoted delimiters, quoted newlines and ragged rows retain present cells. |
| XML | Root name, attributes, text/comment values and original child-element byte slices. Includes SVG, RSS/Atom, XHTML, XML property lists, KML/GPX and Office XML. External entities are not resolved. Child slices retain their lexical bytes; inherited namespace declarations remain in the original source rather than being invented in fragments. |
| YAML | Documents, mappings, sequences and decoded scalar contents; binary scalars expose decoded bytes. Compound children are self-contained YAML. Finite aliases are expanded; cyclic aliases, duplicate mapping keys, expansions over 100000 nodes and nesting over 128 levels produce errors. |
| TOML | Section-qualified values as typed JSON, including arrays and inline tables; table headings also remain visible. Dotted inline keys retain their structure. Date/time and nonfinite numbers use JSON strings. Invalid keys, definitions and values are rejected. |
| INI | Section-qualified key/value entries. `=` and `:` delimit keys; blank lines and whole-line `#`/`;` comments are ignored. Values are trimmed literal text. Repeated keys remain independent. |
| Java properties | Keys and decoded values using the ISO-8859-1 InputStream convention, continuations, escaped delimiters, Unicode escapes and surrogate pairs. Repeated keys remain independent. |
| dotenv | Literal assignments, optional `export`, single/double quotes and multiline quoted values. Double quotes decode newline, return, tab, quote and backslash escapes. Unquoted comments start with whitespace plus `#`. Variables and command substitutions are never expanded. |
| Email / MIME | RFC 5322 headers and decoded body/attachment bytes. Handles multipart nesting, Base64, quoted-printable and nested messages; filenames and media types select downstream filters. Multipart nesting over 64 levels is rejected. |
| mbox | Independent messages after envelope removal and mboxrd `>From` unquoting, each using the email filter. |
| Base64 | Standard and URL-safe Base64, padded or unpadded, with whitespace ignored. Invalid symbols and nonzero padding bits are rejected. |
| Hex | Decoded byte pairs, ignoring whitespace. Odd lengths and invalid digits are rejected. |
| URL encoded | Query/form field values, with percent and `+` decoding. Repeated names have separate handles. |
| JWT | Decoded compact JWS JSON header, claims and signature bytes. Signature verification is explicitly not performed. Encrypted JWE is not decoded. |
| PEM | Each validated BEGIN/END block's decoded bytes, normally routed to ASN.1. Legacy-encrypted or OpenSSH private-key payloads remain raw bytes. No decryption is attempted. |
| ASN.1 | DER classes, tags, constructed children and scalar content bytes. Certificates and unencrypted key structures can be traversed without assuming a schema. Indefinite BER lengths are not DER and are rejected. |
| Protocol Buffers | Numbered wire fields: varints, fixed32, fixed64 and exact length-delimited bytes. No field names, signedness, float types or nested schema are guessed. Deprecated groups are explicitly rejected. |
| SQLite | Tables → rows → cell bytes, including exact BLOBs, text bytes, numeric values and null. Identifiers are quoted. Reads use a separate in-memory SQLite engine with query-only mode, defensive settings, no trusted schema and memory temp storage. The input snapshot is not modified. A complete standalone database is required; separate WAL/journal files and encrypted databases are not combined or decrypted. Unsupported virtual tables report their read error. |
| Lance | Version 2 rows as independently expandable JSON objects, plus row count, schema, field metadata, exact column-metadata protobufs and global buffers. Lance's source-built Rust reader decodes the column pages. The default view shows this structure without dumping the file bytes. Complete source remains available under **[source bytes]**. |
| Byte bundle | Lossless generated containers used for SQLite tables and rows. Cell data is Base64 in the serialized container and becomes actual byte children when opened. Provider/action descriptors are rejected. |
| RIFF | RIFF/RIFX and LIST chunks, including WAVE, AVI and WebP, with padding and declared-length validation. Chunk payloads retain their original bytes. |
| MP4 / ISO media | ISO base media boxes, including extended sizes and nested structural boxes, for MP4, MOV, M4A, AVIF and HEIF. Unknown box payloads stay raw; media is not transcoded. |
| Text, HTML | Text bytes and extracted links, or HTML title/source/links without execution. |
| Image, PDF | Existing browser preview plus supported dimensions/PDF metadata and links. |
| Diff | Unified diff hunks and lines. |

All readers use complete document bytes in memory, as the existing ZIP and JSON readers do. Parsing failure produces a visible error and retains original input. Unsupported/proprietary formats still have original bytes, download, paged preview and selectable filters; this catalog does not claim a structured parser for every format. Binary property lists, legacy Office compound files, 7z/RAR and schema-dependent formats need additional readers.

## Shared catalog and containers

`FilterInfo` contains `driver` (embedded runnable name), `kind` (resource routing kind), `description` (user help), `extensions` (recognized filename suffixes) and `text` (whether the encoded input is textual). `FilterCatalog` lists all byte filters. `AdditionalFilters` lists the additional readers dispatched by `filters.Parser`; `KindForName` uses the full catalog for disk, Git, ZIP, TAR, S3 and Kubernetes entries.

`ByteBundle` is JSON with `format` equal to `datatree-bytes-v1` and an `entries` array. Each `ByteEntry` contains `name`, `kind` and `data` (standard JSON Base64 bytes). Only ordinary byte kinds are allowed. Unknown fields, trailing data and invalid format identifiers are errors. Repeated names retain their own data and internal keys. Original bundle bytes remain available like any other source.

To add a reader, add its catalog entry and parser dispatch, return ordinary resources and byte payloads, and cover valid input, malformed input and downstream composition. The host, upload selector and gallery obtain available byte kinds from the catalog. Existing XSH launchers build and test the complete embedded application without additional runtime tools.
