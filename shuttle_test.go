package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"testing"
	"time"

	. "launchpad.net/gocheck"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func Test(t *testing.T) { TestingT(t) }

type BasicSuite struct {
	servers []*testServer
	service *Service
}

var _ = Suite(&BasicSuite{})

func (s *BasicSuite) SetUpTest(c *C) {
	// start 4 possible backend servers
	ports := []string{"9001", "9002", "9003", "9004"}
	for _, p := range ports {
		server, err := NewTestServer("127.0.0.1:"+p, c)
		if err != nil {
			c.Fatal(err)
		}
		s.servers = append(s.servers, server)
	}

	svcCfg := ServiceConfig{
		Name: "testService",
		Addr: "127.0.0.1:9999",
	}
	s.service = NewService(svcCfg)
	if err := s.service.Start(); err != nil {
		c.Fatal(err)
	}
}

// Add a default backend for the next server we have running
func (s *BasicSuite) AddBackend(check bool, c *C) {
	next := len(s.service.Backends)
	if next >= len(s.servers) {
		c.Fatal("no more servers")
	}

	name := fmt.Sprintf("backend_%d", next)
	cfg := BackendConfig{
		Name: name,
		Addr: s.servers[next].addr,
	}

	if check {
		cfg.Check = cfg.Addr
	}

	s.service.Add(NewBackend(cfg))
}

// shutdown our backend servers
func (s *BasicSuite) TearDownTest(c *C) {
	for _, s := range s.servers {
		s.Stop()
	}

	Registry.Remove(s.service.Name)
}

// Connect to address, and check response after write.
func checkResp(addr, expected string, c *C) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		c.Fatal(err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "testing\n"); err != nil {
		c.Fatal(err)
	}

	buff := make([]byte, 1024)
	n, err := conn.Read(buff)
	if err != nil {
		c.Fatal(err)
	}

	resp := string(buff[:n])
	c.Assert(resp, Matches, expected)
}

func (s *BasicSuite) TestSingleBackend(c *C) {
	s.AddBackend(false, c)

	checkResp(s.service.Addr, s.servers[0].addr, c)
}

func (s *BasicSuite) TestRoundRobin(c *C) {
	s.AddBackend(false, c)
	s.AddBackend(false, c)

	checkResp(s.service.Addr, s.servers[0].addr, c)
	checkResp(s.service.Addr, s.servers[1].addr, c)
	checkResp(s.service.Addr, s.servers[0].addr, c)
	checkResp(s.service.Addr, s.servers[1].addr, c)
}

func (s *BasicSuite) TestLeastConn(c *C) {
	s.service.SetBalance("LC")
	s.AddBackend(false, c)
	s.AddBackend(false, c)

	// tie up 4 connections to the backends
	for i := 0; i < 4; i++ {
		conn, e := net.Dial("tcp", s.service.Addr)
		if e != nil {
			c.Fatal(e)
		}
		defer conn.Close()
	}

	s.AddBackend(false, c)

	checkResp(s.service.Addr, s.servers[2].addr, c)
	checkResp(s.service.Addr, s.servers[2].addr, c)
}

func (s *BasicSuite) TestFailedCheck(c *C) {
	s.service.Inter = 1
	s.service.Fall = 1
	s.AddBackend(true, c)

	stats := s.service.Stats()
	c.Assert(stats.Backends[0].Up, Equals, true)

	s.servers[0].Stop()
	time.Sleep(1200 * time.Millisecond)

	stats = s.service.Stats()
	c.Assert(stats.Backends[0].Up, Equals, false)
}

func (s *BasicSuite) TestUpdateBackend(c *C) {
	s.AddBackend(true, c)

	cfg := s.service.Config()
	backendCfg := cfg.Backends[0]

	c.Assert(backendCfg.Check, Matches, backendCfg.Addr)

	backendCfg.Check = ""
	s.service.Add(NewBackend(backendCfg))

	cfg = s.service.Config()
	c.Assert(len(cfg.Backends), Equals, 1)
	c.Assert(cfg.Backends[0].Check, Matches, "")
}
