package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

var (
	Registry = ServiceRegistry{
		svcs: make(map[string]*Service, 0),
	}
)

// TODO: updating the Service left a hidden backend

type Backend struct {
	sync.Mutex
	Name   string
	Addr   string
	Check  string
	Up     bool
	Weight uint64
	Sent   uint64
	Rcvd   uint64
	Errors uint64
	Conns  int64
	Active int64

	// these are loaded from the service, se a backend doesn't need to acces
	// the service struct at all.
	dialTimeout time.Duration
	rwTimeout   time.Duration
}

// The json stats we return for the backend
type BackendStat struct {
	Name   string `json:"name"`
	Addr   string `json:"address"`
	Check  string `json:"check_address"`
	Up     bool   `json:"up"`
	Weight uint64 `json:"weight"`
	Sent   uint64 `json:"sent"`
	Rcvd   uint64 `json:"received"`
	Errors uint64 `json:"errors"`
	Conns  int64  `json:"connections"`
	Active int64  `json:"active"`
}

// The subset of fields we load and serialize for config.
type BackendConfig struct {
	Name   string `json:"name"`
	Addr   string `json:"address"`
	Check  string `json:"check_address"`
	Weight uint64 `json:"weight"`
}

func NewBackend(cfg BackendConfig) *Backend {
	b := &Backend{
		Name:   cfg.Name,
		Addr:   cfg.Addr,
		Check:  cfg.Check,
		Weight: cfg.Weight,
	}
	return b
}

// Copy the backend state into a BackendStat struct.
// We probably don't need atomic loads for the live stats here.
func (b *Backend) Stats() BackendStat {
	b.Lock()
	defer b.Unlock()

	stats := BackendStat{
		Name:   b.Name,
		Addr:   b.Addr,
		Check:  b.Check,
		Up:     b.Up,
		Weight: b.Weight,
		Sent:   b.Sent,
		Rcvd:   b.Rcvd,
		Errors: b.Errors,
		Conns:  b.Conns,
		Active: b.Active,
	}

	return stats
}

// Return the struct for marshaling into a json config
func (b *Backend) Config() BackendConfig {
	b.Lock()
	defer b.Unlock()

	cfg := BackendConfig{
		Name:   b.Name,
		Addr:   b.Addr,
		Check:  b.Check,
		Weight: b.Weight,
	}

	return cfg
}

// Backends and Servers Stringify themselves directly into their config format.
func (b Backend) String() string {
	j, err := json.MarshalIndent(b.Stats(), "", "  ")
	if err != nil {
		log.Println("Backend JSON error:", err)
		return ""
	}
	return string(j)
}

type Service struct {
	sync.Mutex
	Name          string
	Addr          string
	Backends      []*Backend
	Balance       string
	Inter         uint64
	ErrLim        uint64
	Fall          uint64
	Rise          uint64
	ClientTimeout time.Duration
	ServerTimeout time.Duration
	DialTimeout   time.Duration
	Sent          uint64
	Rcvd          uint64
	Errors        uint64

	// Next returns the backend to be used for a new connection according our
	// load balancing algorithm
	next func() *Backend
	// the last backend we used and the number of times we used it
	lastBackend int
	lastCount   int

	// Each Service owns it's own netowrk listener
	listener net.Listener
}

// Stats returned about a service
type ServiceStat struct {
	Name          string        `json:"name"`
	Addr          string        `json:"address"`
	Backends      []BackendStat `json:"backends"`
	Balance       string        `json:"balance"`
	Inter         uint64        `json:"check_interval"`
	ErrLim        uint64        `json:"error_limit"`
	Fall          uint64        `json:"fall"`
	Rise          uint64        `json:"rise"`
	ClientTimeout uint64        `json:"client_timeout"`
	ServerTimeout uint64        `json:"server_timeout"`
	DialTimeout   uint64        `json:"connect_timeout"`
	Sent          uint64        `json:"sent"`
	Rcvd          uint64        `json:"received"`
	Errors        uint64        `json:"errors"`
}

// Subset of service fields needed for configuration.
type ServiceConfig struct {
	Name          string          `json:"name"`
	Addr          string          `json:"address"`
	Backends      []BackendConfig `json:"backends"`
	Balance       string          `json:"balance"`
	Inter         uint64          `json:"check_interval"`
	ErrLim        uint64          `json:"error_limit"`
	Fall          uint64          `json:"fall"`
	Rise          uint64          `json:"rise"`
	ClientTimeout uint64          `json:"client_timeout"`
	ServerTimeout uint64          `json:"server_timeout"`
	DialTimeout   uint64          `json:"connect_timeout"`
}

// Create a Service from a config struct
func NewService(cfg ServiceConfig) *Service {
	s := &Service{
		Name:          cfg.Name,
		Addr:          cfg.Addr,
		Balance:       cfg.Balance,
		Inter:         cfg.Inter,
		ErrLim:        cfg.ErrLim,
		Fall:          cfg.Fall,
		Rise:          cfg.Rise,
		ClientTimeout: time.Duration(cfg.ClientTimeout) * time.Second,
		ServerTimeout: time.Duration(cfg.ServerTimeout) * time.Second,
		DialTimeout:   time.Duration(cfg.DialTimeout) * time.Second,
	}

	for _, b := range cfg.Backends {
		s.Backends = append(s.Backends, NewBackend(b))
	}

	return s
}

func (s *Service) Stats() ServiceStat {
	s.Lock()
	defer s.Unlock()

	stats := ServiceStat{
		Name:          s.Name,
		Addr:          s.Addr,
		Balance:       s.Balance,
		Inter:         s.Inter,
		ErrLim:        s.ErrLim,
		Fall:          s.Fall,
		Rise:          s.Rise,
		ClientTimeout: uint64(s.ClientTimeout / time.Second),
		ServerTimeout: uint64(s.ServerTimeout / time.Second),
		DialTimeout:   uint64(s.DialTimeout / time.Second),
	}

	for _, b := range s.Backends {
		stats.Backends = append(stats.Backends, b.Stats())
		stats.Sent += b.Sent
		stats.Rcvd += b.Rcvd
		stats.Errors += b.Errors
	}

	return stats
}

func (s *Service) Config() ServiceConfig {
	s.Lock()
	defer s.Unlock()

	config := ServiceConfig{
		Name:          s.Name,
		Addr:          s.Addr,
		Balance:       s.Balance,
		Inter:         s.Inter,
		ErrLim:        s.ErrLim,
		Fall:          s.Fall,
		Rise:          s.Rise,
		ClientTimeout: uint64(s.ClientTimeout / time.Second),
		ServerTimeout: uint64(s.ServerTimeout / time.Second),
		DialTimeout:   uint64(s.DialTimeout / time.Second),
	}
	for _, b := range s.Backends {
		config.Backends = append(config.Backends, b.Config())
	}

	return config
}

// Fill out and verify service
func (s *Service) Start() (err error) {
	s.Lock()
	defer s.Unlock()

	switch s.Balance {
	case "":
		s.Balance = "RR"
		fallthrough
	case "RR":
		s.next = s.roundRobin
	case "LC":
		s.next = s.leastConn
	default:
		return fmt.Errorf("invalid balancing algorithm")
	}

	s.listener, err = newTimeoutListener(s.Addr, s.ClientTimeout)
	if err != nil {
		return err
	}

	if s.Backends == nil {
		s.Backends = make([]*Backend, 0)
	}

	go s.run()
	Registry.Add(s)

	return nil
}

func (s Service) String() string {
	j, err := json.MarshalIndent(s.Stats(), "", "  ")
	if err != nil {
		log.Println("Service JSON error:", err)
		return ""
	}
	return string(j)
}

func (s *Service) Get(name string) *Backend {
	s.Lock()
	defer s.Unlock()

	for _, b := range s.Backends {
		if b.Name == name {
			return b
		}
	}
	return nil
}

// Add a backend to this service
func (s *Service) Add(backend *Backend) {
	s.Lock()
	defer s.Unlock()

	for _, b := range s.Backends {
		if b.Name == backend.Name {
			return
		}
	}

	backend.Up = true
	backend.rwTimeout = s.ServerTimeout
	s.Backends = append(s.Backends, backend)
}

// Remove a Backend by name
func (s *Service) Remove(name string) *Backend {
	s.Lock()
	defer s.Unlock()

	for i, b := range s.Backends {
		if b.Name == name {
			last := len(s.Backends) - 1
			deleted := b
			s.Backends[i], s.Backends[last] = s.Backends[last], nil
			s.Backends = s.Backends[:last]
			return deleted
		}
	}
	return nil
}

// Start the Service's Accept loop
func (s *Service) run() {
	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				if err := err.(*net.OpError); err.Temporary() {
					continue
				}
				// we must be getting shut down
				return
			}

			backend := s.next()
			if backend == nil {
				log.Println("error: no backend for", s.Name)
				conn.Close()
				continue
			}

			go backend.Proxy(conn)
		}
	}()
}

// Stop the Service's Accept loop by closing the Listener
func (s *Service) Stop() {
	s.Lock()
	defer s.Unlock()

	err := s.listener.Close()
	if err != nil {
		log.Println(err)
	}
}

// ServiceRegistry is a global container for all configured services.
type ServiceRegistry struct {
	sync.Mutex
	svcs map[string]*Service
}

func (s *ServiceRegistry) Add(svc *Service) {
	s.Lock()
	defer s.Unlock()

	s.svcs[svc.Name] = svc
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
	for _, v := range s.svcs {
		configs = append(configs, v.Config())
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
