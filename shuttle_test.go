package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"testing"
	"time"

	. "launchpad.net/gocheck"
)

func init() {
	// log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(ioutil.Discard)
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

	if err := Registry.AddService(svcCfg); err != nil {
		c.Fatal(err)
	}

	s.service = Registry.GetService(svcCfg.Name)
}

// Add a default backend for the next server we have running
func (s *BasicSuite) AddBackend(c *C) {
	next := len(s.service.Backends)
	if next >= len(s.servers) {
		c.Fatal("no more servers")
	}

	name := fmt.Sprintf("backend_%d", next)
	cfg := BackendConfig{
		Name:      name,
		Addr:      s.servers[next].addr,
		CheckAddr: s.servers[next].addr,
	}

	s.service.add(NewBackend(cfg))
}

// shutdown our backend servers
func (s *BasicSuite) TearDownTest(c *C) {
	for _, s := range s.servers {
		s.Stop()
	}

	err := Registry.RemoveService(s.service.Name)
	if err != nil {
		c.Fatalf("could not remove service '%s': %s", s.service.Name, err)
	}
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
	c.Assert(resp, Equals, expected)
}

func (s *BasicSuite) TestSingleBackend(c *C) {
	s.AddBackend(c)

	checkResp(s.service.Addr, s.servers[0].addr, c)
}

func (s *BasicSuite) TestRoundRobin(c *C) {
	s.AddBackend(c)
	s.AddBackend(c)

	checkResp(s.service.Addr, s.servers[0].addr, c)
	checkResp(s.service.Addr, s.servers[1].addr, c)
	checkResp(s.service.Addr, s.servers[0].addr, c)
	checkResp(s.service.Addr, s.servers[1].addr, c)
}

func (s *BasicSuite) TestLeastConn(c *C) {
	// this assignment triggers race detection
	s.service.next = s.service.leastConn

	s.AddBackend(c)
	s.AddBackend(c)

	// tie up 4 connections to the backends
	for i := 0; i < 4; i++ {
		conn, e := net.Dial("tcp", s.service.Addr)
		if e != nil {
			c.Fatal(e)
		}
		defer conn.Close()
	}

	s.AddBackend(c)

	checkResp(s.service.Addr, s.servers[2].addr, c)
	checkResp(s.service.Addr, s.servers[2].addr, c)
}

// Test health check by taking down a server from a configured backend
func (s *BasicSuite) TestFailedCheck(c *C) {
	s.service.CheckInterval = 1
	s.service.Fall = 1
	s.AddBackend(c)

	stats := s.service.Stats()
	c.Assert(stats.Backends[0].Up, Equals, true)

	// Stop the server, and see if the backend shows Down after our check
	// interval.
	s.servers[0].Stop()
	time.Sleep(1200 * time.Millisecond)

	stats = s.service.Stats()
	c.Assert(stats.Backends[0].Up, Equals, false)
	c.Assert(stats.Backends[0].CheckFail, Equals, 1)

	// now try and connect to the service
	conn, err := net.Dial("tcp", s.service.Addr)
	if err != nil {
		// we should still get an initial connection
		c.Fatal(err)
	}

	b := make([]byte, 1024)
	n, err := conn.Read(b)
	if n != 0 || err != io.EOF {
		c.Fatal("connection should have been closed")
	}
}

// Update a backend in place
func (s *BasicSuite) TestUpdateBackend(c *C) {
	s.service.CheckInterval = 1
	s.service.Fall = 1
	s.AddBackend(c)

	cfg := s.service.Config()
	backendCfg := cfg.Backends[0]

	c.Assert(backendCfg.CheckAddr, Equals, backendCfg.Addr)

	backendCfg.CheckAddr = ""
	s.service.add(NewBackend(backendCfg))

	// see if the config reflects the new backend
	cfg = s.service.Config()
	c.Assert(len(cfg.Backends), Equals, 1)
	c.Assert(cfg.Backends[0].CheckAddr, Equals, "")

	// Stopping the server should not take down the backend
	// since there is no longer a Check address.
	s.servers[0].Stop()
	time.Sleep(1200 * time.Millisecond)

	stats := s.service.Stats()
	c.Assert(stats.Backends[0].Up, Equals, true)
	// should have been no check failures
	c.Assert(stats.Backends[0].CheckFail, Equals, 0)
}

// Test removal of a single Backend from a service with multiple.
func (s *BasicSuite) TestRemoveBackend(c *C) {
	s.AddBackend(c)
	s.AddBackend(c)

	stats, err := Registry.ServiceStats("testService")
	if err != nil {
		c.Fatal(err)
	}

	c.Assert(len(stats.Backends), Equals, 2)

	backend1 := stats.Backends[0].Name

	err = Registry.RemoveBackend("testService", backend1)
	if err != nil {
		c.Fatal(err)
	}

	stats, err = Registry.ServiceStats("testService")
	if err != nil {
		c.Fatal(err)
	}

	c.Assert(len(stats.Backends), Equals, 1)

	_, err = Registry.BackendStats("testService", backend1)
	c.Assert(err, Equals, ErrNoBackend)
}

func (s *BasicSuite) TestUpdateService(c *C) {
	svcCfg := ServiceConfig{
		Name: "Update",
		Addr: "127.0.0.1:9324",
	}

	if err := Registry.AddService(svcCfg); err != nil {
		c.Fatal(err)
	}

	svc := Registry.GetService("Update")
	if svc == nil {
		c.Fatal(ErrNoService)
	}

	svcCfg = ServiceConfig{
		Name: "Update",
		Addr: "127.0.0.1:9425",
	}

	// Make sure we can't add it through AddService
	if err := Registry.AddService(svcCfg); err == nil {
		c.Fatal(err)
	}

	// Now update the service
	if err := Registry.UpdateService(svcCfg); err != nil {
		c.Fatal(err)
	}

	svc = Registry.GetService("Update")
	if svc == nil {
		c.Fatal(ErrNoService)
	}
	c.Assert(svc.Addr, Equals, "127.0.0.1:9425")

	if err := Registry.RemoveService("Update"); err != nil {
		c.Fatal(err)
	}
}
