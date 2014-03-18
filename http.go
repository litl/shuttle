package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func getService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	svc := Services.Get(vars["service"])
	if svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	fmt.Fprintln(w, svc)
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

	svc := NewService(svcCfg)

	if e := svc.Start(); e != nil {
		// we can probably distinguish between 4xx and 5xx errors here at some point.
		log.Println(err)
		http.Error(w, e.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintln(w, svc)
}

func deleteService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	svc := Services.Remove(vars["service"])
	if svc == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	fmt.Fprintln(w, svc)
}

func listServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fmt.Fprintln(w, Services.String())
}

func getBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	svc := Services.Get(vars["service"])
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

	svc := Services.Get(vars["service"])
	if svc == nil {
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

	svc.Add(backend)
}

func deleteBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := Services.Get(vars["service"])
	if service == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	backend := service.Remove(vars["backend"])
	if backend == nil {
		http.Error(w, "backend not found", http.StatusNotFound)
		return
	}
}
