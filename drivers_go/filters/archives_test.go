package filters

import (
	"archive/tar"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func TestCompressionAndNestedTar(t *testing.T) {
	payload := []byte("actual data\x00")
	for _, name := range []string{"gzip_driver", "zlib_driver", "deflate_driver", "xz_driver", "zstd_driver"} {
		t.Run(name, func(t *testing.T) {
			var buffer bytes.Buffer
			var writer io.WriteCloser
			var err error
			switch name {
			case "gzip_driver":
				writer = gzip.NewWriter(&buffer)
			case "zlib_driver":
				writer = zlib.NewWriter(&buffer)
			case "deflate_driver":
				writer, err = flate.NewWriter(&buffer, flate.DefaultCompression)
			case "xz_driver":
				writer, err = xz.NewWriter(&buffer)
			case "zstd_driver":
				writer, err = zstd.NewWriter(&buffer)
			default:
				t.Fatal("unexpected compressor")
			}
			switch {
			case err != nil:
				t.Fatal(err)
			default:
			}
			_, err = writer.Write(payload)
			switch {
			case err != nil:
				t.Fatal(err)
			default:
			}
			switch err := writer.Close(); {
			case err != nil:
				t.Fatal(err)
			default:
			}
			parser, _ := Parser(name)
			resources, children, err := parser(buffer.Bytes())
			switch {
			case err != nil || len(resources) != 1:
				t.Fatalf("decode: %+v %v", resources, err)
			default:
			}
			switch {
			case !bytes.Equal(children[resources[0].Key], payload):
				t.Fatal("decompression changed bytes")
			default:
			}
			_, _, err = parser(buffer.Bytes()[:buffer.Len()-2])
			switch {
			case err == nil:
				t.Fatal("accepted truncated compressed stream")
			default:
			}
		})
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for _, value := range []string{"first", "second"} {
		switch err := writer.WriteHeader(&tar.Header{Name: "../same.json", Mode: 0600, Size: int64(len(value))}); {
		case err != nil:
			t.Fatal(err)
		default:
		}
		switch _, err := writer.Write([]byte(value)); {
		case err != nil:
			t.Fatal(err)
		default:
		}
	}
	switch err := writer.Close(); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	resources, children, err := listTar(archive.Bytes())
	switch {
	case err != nil || len(resources) != 2:
		t.Fatalf("TAR: %+v %v", resources, err)
	case resources[0].Key == resources[1].Key || resources[0].Kind != "json" || string(children[resources[0].Key]) != "first" || string(children[resources[1].Key]) != "second":
		t.Fatal("TAR duplicates lost data")
	default:
	}
	var compressed bytes.Buffer
	zipper := gzip.NewWriter(&compressed)
	zipper.Name = "contents.tar"
	switch _, err := zipper.Write(archive.Bytes()); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch err := zipper.Close(); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	resources, children, err = listGzip(compressed.Bytes())
	switch {
	case err != nil || resources[0].Kind != "tar":
		t.Fatalf("GZIP did not classify TAR: %+v %v", resources, err)
	case !bytes.Equal(children[resources[0].Key], archive.Bytes()):
		t.Fatal("GZIP changed TAR bytes")
	default:
	}
}

func TestBzip2Fixture(t *testing.T) {
	// Fixture generated with Python's standard bz2 compressor, no runtime tool.
	raw, err := base64.StdEncoding.DecodeString("QlpoOTFBWSZTWWCdcDsAAAHRgEAAQAAsBAYAIAAiAzSEMCNZ0jMqHi7kinChIME64HY=")
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	resources, children, err := listBzip2(raw)
	switch {
	case err != nil:
		t.Fatal(err)
	case string(children[resources[0].Key]) != "actual data\x00":
		t.Fatal("BZIP2 changed bytes")
	default:
	}
}

func TestBinaryContainers(t *testing.T) {
	riff := append([]byte("RIFF\x00\x00\x00\x00WAVEdata\x04\x00\x00\x00"), []byte{1, 2, 0, 255}...)
	binary.LittleEndian.PutUint32(riff[4:8], uint32(len(riff)-8))
	resources, children, err := listRIFF(riff)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch {
	case !bytes.Equal(childBytes(t, resources, children, "data"), []byte{1, 2, 0, 255}):
		t.Fatal("RIFF lost payload")
	default:
	}
	box := []byte{0, 0, 0, 12, 'm', 'd', 'a', 't', 1, 2, 0, 255}
	moov := append([]byte{0, 0, 0, 20, 'm', 'o', 'o', 'v'}, box...)
	resources, children, err = listMP4(moov)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	resources, children, err = listMP4(childBytes(t, resources, children, "moov"))
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch {
	case !bytes.Equal(childBytes(t, resources, children, "mdat"), []byte{1, 2, 0, 255}):
		t.Fatal("MP4 lost payload")
	default:
	}
}
