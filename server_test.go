package main

import (
	"io"
	"log"
	"net"
)

type testServer struct {
	addr     string
	listener net.Listener
}

// Start a tcp server which responds with sig after every read.
func NewTestServer(addr, sig string) (*testServer, error) {
	s := &testServer{
		addr: addr,
	}

	var err error
	s.listener, err = net.Listen("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				log.Printf("test server '%s' exiting", sig)
				return
			}

			log.Println("received connected on test server:", sig)
			go func() {
				defer conn.Close()
				buff := make([]byte, 1024)
				if _, err := conn.Read(buff); err != nil {
					log.Printf("test server '%s' error: %s", sig, err)
					return
				}
				if _, err := io.WriteString(conn, sig); err != nil {
					log.Printf("test server '%s' error: %s", sig, err)
					return
				}
				// make one more read to wait until EOF
				conn.Read(buff)
			}()
		}
	}()
	return s, nil
}

func (s *testServer) Stop() {
	s.listener.Close()
}
