package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

func loadConfig() {
	for _, cfgPath := range []string{stateConfig, defaultConfig} {
		if cfgPath == "" {
			continue
		}

		cfgData, err := ioutil.ReadFile(cfgPath)
		if err != nil {
			log.Println("error reading config:", err)
			continue
		}

		var svcs []ServiceConfig
		err = json.Unmarshal(cfgData, &svcs)
		if err != nil {
			log.Println("config error:", err)
			continue
		}

		for _, svcCfg := range svcs {
			svc := NewService(svcCfg)
			if e := svc.Start(); e != nil {
				log.Println("service error:", e)
			}
		}
	}
}

func getConfig(w http.ResponseWriter, r *http.Request) {
	cfg := Services.Config()

	jsonCfg, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(jsonCfg)
}
