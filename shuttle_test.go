package main

import (
	"io"
	"log"
	"net"
	"testing"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// Connect to address, and check response after write.
func checkSig(addr, sig string, t *testing.T) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "testing\n"); err != nil {
		t.Fatal(err)
	}

	buff := make([]byte, 1024)
	n, err := conn.Read(buff)
	if err != nil {
		t.Fatal(err)
	}

	resp := string(buff[:n])
	if sig != "" && resp != sig {
		t.Fatal("incorrect reponse:", resp)
	}
}

func TestSingleBackend(t *testing.T) {
	sigString := "single"

	s, err := NewTestServer("127.0.0.1:9876", sigString)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	serviceCfg := ServiceConfig{
		Name: "testService",
		Addr: "127.0.0.1:9999",
	}

	backend := BackendConfig{
		Name: "testBackend",
		Addr: "127.0.0.1:9876",
	}
	serviceCfg.Backends = append(serviceCfg.Backends, backend)

	service := NewService(serviceCfg)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}

	checkSig(serviceCfg.Addr, sigString, t)
	Registry.Remove("testService")
}

func TestRoundRobin(t *testing.T) {
	sigOne := "first server"
	sigTwo := "second server"

	s1, err := NewTestServer("127.0.0.1:9001", sigOne)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Stop()
	s2, err := NewTestServer("127.0.0.1:9002", sigTwo)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()

	serviceCfg := ServiceConfig{
		Name:    "testService",
		Addr:    "127.0.0.1:9000",
		Balance: "RR",
	}

	backend1 := BackendConfig{
		Name: "testBackendOne",
		Addr: "127.0.0.1:9001",
	}
	backend2 := BackendConfig{
		Name: "testBackendTwo",
		Addr: "127.0.0.1:9002",
	}

	serviceCfg.Backends = append(serviceCfg.Backends, backend1, backend2)

	service := NewService(serviceCfg)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}

	checkSig(serviceCfg.Addr, sigOne, t)
	checkSig(serviceCfg.Addr, sigTwo, t)
	checkSig(serviceCfg.Addr, sigOne, t)
	checkSig(serviceCfg.Addr, sigTwo, t)
	Registry.Remove("testService")
}

func TestLeastConn(t *testing.T) {
	sigOne := "first server"
	sigTwo := "second server"
	sigThree := "third server"

	s1, err := NewTestServer("127.0.0.1:9001", sigOne)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Stop()
	s2, err := NewTestServer("127.0.0.1:9002", sigTwo)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()

	s3, err := NewTestServer("127.0.0.1:9003", sigThree)
	if err != nil {
		t.Fatal(err)
	}
	defer s3.Stop()

	serviceCfg := ServiceConfig{
		Name:    "testService",
		Addr:    "127.0.0.1:9000",
		Balance: "LC",
	}

	backend1 := BackendConfig{
		Name: "testBackendOne",
		Addr: "127.0.0.1:9001",
	}
	backend2 := BackendConfig{
		Name: "testBackendTwo",
		Addr: "127.0.0.1:9002",
	}
	backend3 := BackendConfig{
		Name: "testBackendThree",
		Addr: "127.0.0.1:9003",
	}

	// only add the first two
	serviceCfg.Backends = append(serviceCfg.Backends, backend1, backend2)

	service := NewService(serviceCfg)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}

	// tie up 4 connections to the backends
	for i := 0; i < 4; i++ {
		c, e := net.Dial("tcp", serviceCfg.Addr)
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
	}

	// now add a third backend
	service.Add(NewBackend(backend3))
	checkSig(serviceCfg.Addr, sigThree, t)
	checkSig(serviceCfg.Addr, sigThree, t)

	Registry.Remove("testService")
}
