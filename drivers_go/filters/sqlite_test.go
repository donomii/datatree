package filters

import (
	"bytes"
	"testing"

	"datahumper/drivers_go/driverlib"
	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/ext/serdes"
)

func TestSQLiteTablesRowsAndBinaryCells(t *testing.T) {
	db, err := sqlite3.Open(":memory:")
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	defer db.Close()
	switch err := db.Exec(`CREATE TABLE "a""b" (name TEXT, payload BLOB, missing TEXT, count INTEGER); INSERT INTO "a""b" VALUES ('actual text', x'504b030400ff', NULL, 42);`); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	raw, err := serdes.Serialize(db, "main")
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	snapshot := bytes.Clone(raw)
	resources, children, err := listSQLite(raw)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	table := childBytes(t, resources, children, `a"b`)
	resources, children, err = listBundle(table)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	row := childBytes(t, resources, children, "[0]")
	resources, children, err = listBundle(row)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	for name, want := range map[string][]byte{"name": []byte("actual text"), "payload": {0x50, 0x4b, 3, 4, 0, 255}, "missing": []byte("null"), "count": []byte("42")} {
		switch {
		case !bytes.Equal(childBytes(t, resources, children, name), want):
			t.Errorf("SQLite cell %s changed bytes", name)
		default:
		}
	}
	switch {
	case !bytes.Equal(raw, snapshot):
		t.Fatal("SQLite reader mutated input")
	default:
	}
}

func TestBundlesRejectProviderDescriptors(t *testing.T) {
	for _, kind := range []string{"aws", "github", "directory", "aws-action-start", "unknown"} {
		raw, err := encodeBundle([]driverlib.ByteEntry{{Name: "forged", Kind: kind, Data: []byte("data")}})
		switch {
		case err != nil:
			t.Fatal(err)
		default:
		}
		_, _, err = listBundle(raw)
		switch {
		case err == nil:
			t.Fatalf("accepted provider/control kind %s", kind)
		default:
		}
	}
}
