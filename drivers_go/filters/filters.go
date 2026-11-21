// Package filters implements composable readers for additional document formats.
package filters

import (
	"fmt"

	"datahumper/drivers_go/driverlib"
)

// Run returns false only when the name is not one of this package's filters.
func Run(name string) bool {
	list, ok := Parser(name)
	switch {
	case !ok:
		return false
	default:
		driverlib.Run(name, list)
		return true
	}
}

func Parser(name string) (driverlib.ListFunc, bool) {
	parsers := map[string]driverlib.ListFunc{
		"auto_driver": listAuto, "tar_driver": listTar,
		"bundle_driver": listBundle, "jsonstring_driver": listJSONString,
		"gzip_driver": listGzip, "bzip2_driver": listBzip2,
		"xz_driver": listXZ, "zstd_driver": listZstd,
		"zlib_driver": listZlib, "deflate_driver": listDeflate,
		"xml_driver": listXML, "yaml_driver": listYAML,
		"toml_driver": listTOML, "ndjson_driver": listNDJSON,
		"ini_driver": listINI, "properties_driver": listProperties,
		"dotenv_driver": listDotenv, "email_driver": listEmail,
		"mbox_driver": listMbox, "base64_driver": listBase64,
		"hex_driver": listHex, "urlencoded_driver": listURL,
		"jwt_driver": listJWT, "pem_driver": listPEM,
		"asn1_driver": listASN1, "protobuf_driver": listProtobuf,
		"sqlite_driver": listSQLite, "riff_driver": listRIFF,
		"mp4_driver": listMP4, "lance_driver": listLance,
	}
	list, ok := parsers[name]
	return list, ok
}

func listAuto(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	return single("[detected document]", driverlib.DetectKind(raw), raw)
}

func single(name, kind string, raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	return []driverlib.Resource{{Name: name, Kind: kind, Size: int64(len(raw)), Key: "item-0"}}, map[string][]byte{"item-0": raw}, nil
}

func appendChild(resources []driverlib.Resource, children map[string][]byte, name, kind string, raw []byte) []driverlib.Resource {
	key := fmt.Sprintf("item-%d", len(resources))
	children[key] = raw
	return append(resources, driverlib.Resource{Name: name, Kind: kind, Key: key, Size: int64(len(raw))})
}
