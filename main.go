package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

var (
	// Location of the default config.
	// This will not be overwritten by shuttle.
	defaultConfig string

	// Location of the live config which is updated on every state change.
	// The default config is loaded if this file does not exist.
	stateConfig string

	// Listen address for the http server
	listenAddr string
)

func init() {
	flag.StringVar(&listenAddr, "http", "127.0.0.1:9090", "http server address")
	flag.StringVar(&defaultConfig, "config", "", "default config file")
	flag.StringVar(&stateConfig, "state", "", "updated config which reflects the internal state")

	flag.Parse()
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", getStats).Methods("GET")
	r.HandleFunc("/config", getConfig).Methods("GET")
	r.HandleFunc("/{service}", getService).Methods("GET")
	r.HandleFunc("/{service}", postService).Methods("PUT", "POST")
	r.HandleFunc("/{service}", deleteService).Methods("DELETE")
	r.HandleFunc("/{service}/{backend}", getBackend).Methods("GET")
	r.HandleFunc("/{service}/{backend}", postBackend).Methods("PUT", "POST")
	r.HandleFunc("/{service}/{backend}", deleteBackend).Methods("DELETE")
	http.Handle("/", r)

	loadConfig()

	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
