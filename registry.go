package main

import (
	"encoding/json"
	"log"
	"sync"
)

// ServiceRegistry is a global container for all configured services.
type ServiceRegistry struct {
	sync.Mutex
	svcs map[string]*Service
}

func (s *ServiceRegistry) Add(svc *Service) error {
	s.Lock()
	defer s.Unlock()

	s.svcs[svc.Name] = svc
	return svc.Start()
}

func (s *ServiceRegistry) Remove(name string) *Service {
	s.Lock()
	defer s.Unlock()

	svc, ok := s.svcs[name]
	if ok {
		delete(s.svcs, name)
		svc.Stop()
		return svc
	}
	return nil
}

func (s *ServiceRegistry) Get(name string) *Service {
	s.Lock()
	defer s.Unlock()
	return s.svcs[name]
}

func (s *ServiceRegistry) Stats() []ServiceStat {
	s.Lock()
	defer s.Unlock()

	var stats []ServiceStat
	for _, service := range s.svcs {
		stats = append(stats, service.Stats())
	}

	return stats
}

func (s *ServiceRegistry) Config() []ServiceConfig {
	s.Lock()
	defer s.Unlock()

	var configs []ServiceConfig
	for _, service := range s.svcs {
		configs = append(configs, service.Config())
	}

	return configs
}

func (s *ServiceRegistry) Marshal() []byte {
	stats := s.Stats()

	jsonStats, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		log.Println("could not marshal services:", err)
		return nil
	}

	return jsonStats
}

func (s *ServiceRegistry) String() string {
	return string(s.Marshal())
}
