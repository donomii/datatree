package filters

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"

	"datahumper/drivers_go/driverlib"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func listTar(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case len(raw) < 512:
		return nil, nil, fmt.Errorf("cannot read TAR: expected at least one 512-byte header block, got %d bytes", len(raw))
	default:
	}
	reader := tar.NewReader(bytes.NewReader(raw))
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for {
		header, err := reader.Next()
		switch {
		case err == io.EOF:
			return resources, children, nil
		case err != nil:
			return nil, nil, fmt.Errorf("cannot read TAR member %d: %v", len(resources), err)
		default:
		}
		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeSymlink, tar.TypeLink:
			resources = appendChild(resources, children, header.Name+" [link target]", "text", []byte(header.Linkname))
		case tar.TypeReg, tar.TypeRegA:
			data, err := io.ReadAll(reader)
			switch {
			case err != nil:
				return nil, nil, fmt.Errorf("cannot read TAR member %q: %v", header.Name, err)
			default:
				resources = appendChild(resources, children, header.Name, driverlib.KindForName(header.Name), data)
			}
		default:
			resources = append(resources, driverlib.KV(header.Name+" [special entry type]", fmt.Sprint(header.Typeflag)))
		}
	}
}

func decompressed(name string, reader io.Reader, openErr error) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case openErr != nil:
		return nil, nil, fmt.Errorf("cannot open compressed stream %q: %v", name, openErr)
	default:
	}
	data, err := io.ReadAll(reader)
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot decompress %q: expected a complete valid stream: %v", name, err)
	default:
		kind := driverlib.DetectKind(data)
		switch namedKind, known := driverlib.FormatKindForName(name); {
		case known:
			kind = namedKind
		default:
		}
		return single(name, kind, data)
	}
}

func listGzip(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot open GZIP stream: %v", err)
	default:
	}
	defer reader.Close()
	name := reader.Name
	switch name {
	case "":
		name = "[decompressed GZIP]"
	default:
	}
	return decompressed(name, reader, nil)
}

func listBzip2(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	return decompressed("[decompressed BZIP2]", bzip2.NewReader(bytes.NewReader(raw)), nil)
}

func listXZ(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	reader, err := xz.NewReader(bytes.NewReader(raw))
	return decompressed("[decompressed XZ]", reader, err)
}

func listZstd(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	reader, err := zstd.NewReader(bytes.NewReader(raw), zstd.WithDecoderConcurrency(1))
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot open Zstandard stream: %v", err)
	default:
		defer reader.Close()
		return decompressed("[decompressed Zstandard]", reader, nil)
	}
}

func listZlib(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(raw))
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot open ZLIB stream: %v", err)
	default:
		defer reader.Close()
		return decompressed("[decompressed ZLIB]", reader, nil)
	}
}

func listDeflate(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	reader := flate.NewReader(bytes.NewReader(raw))
	defer reader.Close()
	return decompressed("[decompressed DEFLATE]", reader, nil)
}
