package main

import (
	"encoding/json"
	"net/http"

	"datahumper/drivers_go/driverlib"
	"datahumper/drivers_go/filters"
)

func extendedFilterNames(names []string) []string {
	for _, filter := range driverlib.AdditionalFilters() {
		names = append(names, filter.Driver)
	}
	return names
}

func additionalDriverForKind(kind string) (string, bool) {
	for _, filter := range driverlib.AdditionalFilters() {
		switch filter.Kind {
		case kind:
			return filter.Driver, true
		default:
		}
	}
	return "text_driver", true
}

func runAdditionalFilter(name string) {
	switch filters.Run(name) {
	case true:
	default:
		logUnknownEmbeddedDriver(name)
	}
}

func mountFilterCatalog(mux *http.ServeMux) {
	mux.HandleFunc("/api/formats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(driverlib.FilterCatalog())
	})
}
