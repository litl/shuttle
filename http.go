package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func getConfig(w http.ResponseWriter, r *http.Request) {
	cfg := Registry.Config()

	cfgJson, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(cfgJson)
	w.Write([]byte("\n"))
}

func getStats(w http.ResponseWriter, r *http.Request) {
	w.Write(Registry.Marshal())
	w.Write([]byte("\n"))
}

func getService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	service := Registry.Get(vars["service"])
	if service == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	fmt.Fprintln(w, service)
}

func postService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	svcCfg := ServiceConfig{Name: vars["service"]}
	err = json.Unmarshal(body, &svcCfg)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	service := NewService(svcCfg)

	if e := service.Start(); e != nil {
		// we can probably distinguish between 4xx and 5xx errors here at some point.
		log.Println(err)
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}

	go writeStateConfig()
	fmt.Fprintln(w, service)
}

func deleteService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := Registry.Remove(vars["service"])
	if service == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	go writeStateConfig()
	fmt.Fprintln(w, service)
}

func getBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	svc := Registry.Get(vars["service"])
	if svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	backend := svc.Get(vars["backend"])
	if backend == nil {
		http.Error(w, "backend not found", http.StatusNotFound)
		return
	}

	fmt.Fprintln(w, backend)
}

func postBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	service := Registry.Get(vars["service"])
	if service == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	backendCfg := BackendConfig{Name: vars["backend"]}
	err = json.Unmarshal(body, &backendCfg)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	backend := NewBackend(backendCfg)

	service.Add(backend)

	go writeStateConfig()
	fmt.Fprintln(w, service)
}

func deleteBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := Registry.Get(vars["service"])
	if service == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	backend := service.Remove(vars["backend"])
	if backend == nil {
		http.Error(w, "backend not found", http.StatusNotFound)
		return
	}

	go writeStateConfig()
	fmt.Fprintln(w, service)
}
