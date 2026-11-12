package filters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"datahumper/drivers_go/driverlib"
	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/ext/serdes"
)

func listSQLite(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case !bytes.HasPrefix(raw, []byte("SQLite format 3\x00")):
		return nil, nil, fmt.Errorf("cannot browse SQLite: expected a SQLite 3 database header")
	default:
	}
	db, err := sqlite3.Open(":memory:")
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot create isolated SQLite reader: %v", err)
	default:
	}
	defer db.Close()
	switch err := serdes.Deserialize(db, "main", raw); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot read SQLite snapshot (a complete standalone database is required): %v", err)
	default:
	}
	switch _, err := db.Config(sqlite3.DBCONFIG_DEFENSIVE, true); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot enable defensive SQLite browsing: %v", err)
	default:
	}
	switch err := db.Exec("PRAGMA trusted_schema=OFF; PRAGMA query_only=ON; PRAGMA temp_store=MEMORY;"); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot enforce read-only SQLite browsing: %v", err)
	default:
	}
	statement, _, err := db.Prepare("SELECT name, sql FROM sqlite_schema WHERE type='table' ORDER BY name")
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot list SQLite tables: %v", err)
	default:
	}
	defer statement.Close()
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for statement.Step() {
		name, schema := statement.ColumnText(0), statement.ColumnText(1)
		data, err := sqliteTable(db, name, schema)
		switch {
		case err != nil:
			resources = append(resources, driverlib.Resource{Name: fmt.Sprintf("cannot read SQLite table %q: %v", name, err), Kind: "error"})
		default:
			resources = appendChild(resources, children, name, "bundle", data)
		}
	}
	switch err := statement.Err(); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot finish listing SQLite tables: %v", err)
	default:
		return resources, children, nil
	}
}

func sqliteTable(db *sqlite3.Conn, name, schema string) ([]byte, error) {
	statement, _, err := db.Prepare(`SELECT * FROM "` + strings.ReplaceAll(name, `"`, `""`) + `"`)
	switch {
	case err != nil:
		return nil, err
	default:
	}
	defer statement.Close()
	entries := []driverlib.ByteEntry{{Name: "[schema]", Kind: "text", Data: []byte(schema)}}
	for row := 0; statement.Step(); row++ {
		var cells []driverlib.ByteEntry
		for column := 0; column < statement.ColumnCount(); column++ {
			kind := "json"
			var data []byte
			switch statement.ColumnType(column) {
			case sqlite3.NULL:
				data = []byte("null")
			case sqlite3.INTEGER:
				data = []byte(strconv.FormatInt(statement.ColumnInt64(column), 10))
			case sqlite3.FLOAT:
				data = []byte(strconv.FormatFloat(statement.ColumnFloat(column), 'g', -1, 64))
				kind = "text"
			case sqlite3.TEXT:
				data = bytes.Clone(statement.ColumnRawText(column))
				kind = "text"
			case sqlite3.BLOB:
				data = statement.ColumnBlob(column, nil)
				kind = driverlib.DetectKind(data)
			default:
				return nil, fmt.Errorf("unexpected SQLite column type for %s", statement.ColumnName(column))
			}
			cells = append(cells, driverlib.ByteEntry{Name: statement.ColumnName(column), Kind: kind, Data: data})
		}
		data, err := encodeBundle(cells)
		switch {
		case err != nil:
			return nil, err
		default:
			entries = append(entries, driverlib.ByteEntry{Name: fmt.Sprintf("[%d]", row), Kind: "bundle", Data: data})
		}
	}
	switch err := statement.Err(); {
	case err != nil:
		return nil, err
	default:
		return encodeBundle(entries)
	}
}

func encodeBundle(entries []driverlib.ByteEntry) ([]byte, error) {
	return json.Marshal(driverlib.ByteBundle{Format: "datatree-bytes-v1", Entries: entries})
}

func listBundle(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var bundle driverlib.ByteBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&bundle)
	switch {
	case err != nil || bundle.Format != "datatree-bytes-v1":
		return nil, nil, fmt.Errorf("cannot read Datatree byte bundle: expected datatree-bytes-v1: %v", err)
	default:
	}
	var trailing json.RawMessage
	switch err := decoder.Decode(&trailing); err {
	case io.EOF:
	default:
		return nil, nil, fmt.Errorf("cannot read byte bundle: unexpected trailing data: %v", err)
	}
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for _, entry := range bundle.Entries {
		valid := entry.Kind == "raw" || entry.Kind == "file" || entry.Kind == "zip"
		for _, format := range driverlib.FilterCatalog() {
			valid = valid || entry.Kind == format.Kind
		}
		switch valid {
		case false:
			return nil, nil, fmt.Errorf("cannot read byte bundle entry %q: invalid data kind %q", entry.Name, entry.Kind)
		default:
			resources = appendChild(resources, children, entry.Name, entry.Kind, entry.Data)
		}
	}
	return resources, children, nil
}

func listJSONString(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var value string
	switch err := json.Unmarshal(raw, &value); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot decode JSON string: %v", err)
	default:
		return single("[decoded string]", driverlib.DetectKind([]byte(value)), []byte(value))
	}
}
