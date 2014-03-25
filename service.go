package main

import (
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

type Service struct {
	sync.Mutex
	Name          string
	Addr          string
	Backends      []*Backend
	Balance       string
	CheckInterval uint64
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
	CheckInterval uint64        `json:"check_interval"`
	Fall          uint64        `json:"fall"`
	Rise          uint64        `json:"rise"`
	ClientTimeout uint64        `json:"client_timeout"`
	ServerTimeout uint64        `json:"server_timeout"`
	DialTimeout   uint64        `json:"connect_timeout"`
	Sent          uint64        `json:"sent"`
	Rcvd          uint64        `json:"received"`
	Errors        uint64        `json:"errors"`
	Conns         int64         `json:"connections"`
	Active        int64         `json:"active"`
}

// Subset of service fields needed for configuration.
type ServiceConfig struct {
	Name          string          `json:"name"`
	Addr          string          `json:"address"`
	Backends      []BackendConfig `json:"backends"`
	Balance       string          `json:"balance"`
	CheckInterval uint64          `json:"check_interval"`
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
		CheckInterval: cfg.CheckInterval,
		Fall:          cfg.Fall,
		Rise:          cfg.Rise,
		ClientTimeout: time.Duration(cfg.ClientTimeout) * time.Millisecond,
		ServerTimeout: time.Duration(cfg.ServerTimeout) * time.Millisecond,
		DialTimeout:   time.Duration(cfg.DialTimeout) * time.Millisecond,
	}

	if s.CheckInterval == 0 {
		s.CheckInterval = 2
	}
	if s.Rise == 0 {
		s.Rise = 2
	}
	if s.Fall == 0 {
		s.Fall = 2
	}

	for _, b := range cfg.Backends {
		s.add(NewBackend(b))
	}

	switch cfg.Balance {
	case "RR", "":
		s.next = s.roundRobin
	case "LC":
		s.next = s.leastConn
	default:
		log.Printf("invalid balancing algorithm '%s'", cfg.Balance)
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
		CheckInterval: s.CheckInterval,
		Fall:          s.Fall,
		Rise:          s.Rise,
		ClientTimeout: uint64(s.ClientTimeout / time.Millisecond),
		ServerTimeout: uint64(s.ServerTimeout / time.Millisecond),
		DialTimeout:   uint64(s.DialTimeout / time.Millisecond),
	}

	for _, b := range s.Backends {
		stats.Backends = append(stats.Backends, b.Stats())
		stats.Sent += b.Sent
		stats.Rcvd += b.Rcvd
		stats.Errors += b.Errors
		stats.Conns += b.Conns
		stats.Active += b.Active
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
		CheckInterval: s.CheckInterval,
		Fall:          s.Fall,
		Rise:          s.Rise,
		ClientTimeout: uint64(s.ClientTimeout / time.Millisecond),
		ServerTimeout: uint64(s.ServerTimeout / time.Millisecond),
		DialTimeout:   uint64(s.DialTimeout / time.Millisecond),
	}
	for _, b := range s.Backends {
		config.Backends = append(config.Backends, b.Config())
	}

	return config
}

func (s *Service) String() string {
	return string(marshal(s.Config()))
}

func (s *Service) get(name string) *Backend {
	s.Lock()
	defer s.Unlock()

	for _, b := range s.Backends {
		if b.Name == name {
			return b
		}
	}
	return nil
}

// Add or replace a Backend in this service
func (s *Service) add(backend *Backend) {
	s.Lock()
	defer s.Unlock()

	backend.up = true
	backend.rwTimeout = s.ServerTimeout
	backend.dialTimeout = s.DialTimeout
	backend.checkInterval = time.Duration(s.CheckInterval) * time.Millisecond

	// replace an existing backend if we have it.
	for i, b := range s.Backends {
		if b.Name == backend.Name {
			b.Stop()
			s.Backends[i] = backend
			backend.Start()
			return
		}
	}

	s.Backends = append(s.Backends, backend)

	backend.Start()
}

// Remove a Backend by name
func (s *Service) remove(name string) bool {
	s.Lock()
	defer s.Unlock()

	for i, b := range s.Backends {
		if b.Name == name {
			last := len(s.Backends) - 1
			deleted := b
			s.Backends[i], s.Backends[last] = s.Backends[last], nil
			s.Backends = s.Backends[:last]
			deleted.Stop()
			return true
		}
	}
	return false
}

// Fill out and verify service
func (s *Service) start() (err error) {
	s.Lock()
	defer s.Unlock()

	s.listener, err = newTimeoutListener(s.Addr, s.ClientTimeout)
	if err != nil {
		return err
	}

	if s.Backends == nil {
		s.Backends = make([]*Backend, 0)
	}

	s.run()
	return nil
}

// Start the Service's Accept loop
func (s *Service) run() {
	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				if err, ok := err.(*net.OpError); ok && err.Temporary() {
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

// Stop the Service's Accept loop by closing the Listener,
// and stop all backends for this service.
func (s *Service) stop() {
	s.Lock()
	defer s.Unlock()

	for _, backend := range s.Backends {
		backend.Stop()
	}

	// the service may have been bad, and the listener failed
	if s.listener == nil {
		return
	}

	err := s.listener.Close()
	if err != nil {
		log.Println(err)
	}
}
