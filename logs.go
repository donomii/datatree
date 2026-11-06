package main

import "log"

func logMissingEmbeddedDriver() {
	log.Fatalf("cannot start embedded driver: expected --driver followed by one of the %d built-in driver names, got no driver name", len(driverNames))
}

func logUnknownEmbeddedDriver(driverName string) {
	log.Fatalf("cannot start embedded driver %q: expected one of %v", driverName, driverNames)
}

func logRootDirectoryFailure(err error) {
	log.Fatalf("cannot start Datatree: %v", err)
}

func logRootDirectory(directory string) {
	switch directory {
	case "":
		log.Printf("Datatree is opening the location browser")
	default:
		log.Printf("Datatree is opening directory %s", directory)
	}
}

func logListenFailure(addr string, err error) {
	log.Fatalf("cannot start Datatree: expected to listen on %s, got error: %v", addr, err)
}

func logServingURL(url string) {
	log.Printf("Datatree is running at %s", url)
}

func logBrowserOpenFailure(url string, err error) {
	log.Printf("cannot open Datatree in the browser at %s: %v; open %s manually", url, err, url)
}

func logHTTPServerFailure(url string, err error) {
	log.Fatalf("Datatree server at %s stopped with error: %v", url, err)
}

func logInstalledLayoutSelfTestFailure(err error) {
	log.Fatalf("installed-layout self-test failed: %v", err)
}

func logInstalledLayoutSelfTestPassed() {
	log.Println("installed-layout self-test passed")
}
