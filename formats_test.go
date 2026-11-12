package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"datahumper/drivers_go/driverlib"
)

func TestFormatCatalogAndRouting(t *testing.T) {
	store := NewStore()
	defer store.stopDrivers()
	router := http.NewServeMux()
	mountFilterCatalog(router)
	request := httptest.NewRequest("GET", "/api/formats", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var formats []driverlib.FilterInfo
	switch err := json.Unmarshal(response.Body.Bytes(), &formats); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch {
	case response.Code != 200 || len(formats) != len(store.listDrivers()):
		t.Fatalf("catalog mismatch: %d %d", response.Code, len(formats))
	default:
	}
	seen := map[string]bool{}
	for _, format := range formats {
		switch {
		case seen[format.Driver] || format.Description == "":
			t.Fatalf("invalid format metadata: %+v", format)
		default:
			seen[format.Driver] = true
		}
		driver, ok := driverForKind(format.Kind)
		switch {
		case !ok || driver != format.Driver:
			t.Errorf("kind %s routes to %s expected %s", format.Kind, driver, format.Driver)
		default:
		}
		for _, extension := range format.Extensions {
			filename := "example" + strings.ToUpper(extension)
			switch kind := driverlib.KindForName(filename); kind {
			case format.Kind:
			default:
				t.Errorf("%s routes to %s expected %s", filename, kind, format.Kind)
			}
		}
	}
}
