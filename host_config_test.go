package main

import (
	"reflect"
	"testing"
)

func TestResolveListenAddrUsesFreeLocalPortByDefault(t *testing.T) {
	t.Setenv("DATATREE_ADDR", "")
	got := resolveListenAddr()
	switch {
	case got != "127.0.0.1:0":
		t.Fatalf("default listen address mismatch: got %q expected %q", got, "127.0.0.1:0")
	default:
	}
}

func TestResolveListenAddrUsesConfiguredAddress(t *testing.T) {
	t.Setenv("DATATREE_ADDR", "127.0.0.1:18080")
	got := resolveListenAddr()
	switch {
	case got != "127.0.0.1:18080":
		t.Fatalf("configured listen address mismatch: got %q expected %q", got, "127.0.0.1:18080")
	default:
	}
}

func TestBrowserURLUsesListenerAddress(t *testing.T) {
	got := browserURL("127.0.0.1:54321")
	switch {
	case got != "http://127.0.0.1:54321":
		t.Fatalf("browser URL mismatch: got %q expected %q", got, "http://127.0.0.1:54321")
	default:
	}
}

func TestListDriversReturnsOnlyUploadCompatibleDrivers(t *testing.T) {
	store := NewStore()
	got := store.listDrivers()
	want := []string{"diff_driver", "zip_driver", "text_driver", "json_driver", "image_driver", "pdf_driver", "html_driver", "csv_driver", "tsv_driver"}
	want = append(want, "auto_driver", "bundle_driver", "jsonstring_driver", "tar_driver", "gzip_driver", "bzip2_driver", "xz_driver", "zstd_driver", "zlib_driver", "deflate_driver", "xml_driver", "yaml_driver", "toml_driver", "ndjson_driver", "ini_driver", "properties_driver", "dotenv_driver", "email_driver", "mbox_driver", "base64_driver", "hex_driver", "urlencoded_driver", "jwt_driver", "pem_driver", "asn1_driver", "protobuf_driver", "sqlite_driver", "riff_driver", "mp4_driver")
	switch {
	case !reflect.DeepEqual(got, want):
		t.Fatalf("upload driver list mismatch: got %v expected %v", got, want)
	default:
	}
}

func TestResourceKeyUsesOpaqueKeyWhenPresent(t *testing.T) {
	switch got := resourceKey(Resource{Name: "github", Key: "\x00datatree:virtual:github"}); {
	case got != "\x00datatree:virtual:github":
		t.Fatalf("opaque resource key mismatch: got %q", got)
	default:
	}
	switch got := resourceKey(Resource{Name: "notes.txt"}); {
	case got != "notes.txt":
		t.Fatalf("ordinary resource key mismatch: got %q expected %q", got, "notes.txt")
	default:
	}
}
