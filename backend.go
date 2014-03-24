package main

import (
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

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
	dialTimeout   time.Duration
	rwTimeout     time.Duration
	checkInterval time.Duration
	rise          uint64
	riseCount     uint64
	fall          uint64
	fallCount     uint64

	startCheck sync.Once
	// stop the health-check loop
	stopCheck chan interface{}
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
		Name:      cfg.Name,
		Addr:      cfg.Addr,
		Check:     cfg.Check,
		Weight:    cfg.Weight,
		stopCheck: make(chan interface{}),
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
func (b *Backend) String() string {
	return string(marshal(b.Config))
}

func (b *Backend) Start() {
	go b.startCheck.Do(b.healthCheck)
}

func (b *Backend) Stop() {
	close(b.stopCheck)
}

func (b *Backend) check() {
	if b.Check == "" {
		return
	}

	up := true
	if c, e := net.DialTimeout("tcp", b.Check, b.dialTimeout); e == nil {
		c.Close()
	} else {
		up = false
	}

	b.Lock()
	defer b.Unlock()
	if up {
		b.fallCount = 0
		b.riseCount++
		if b.riseCount >= b.rise {
			b.Up = true
		}
	} else {
		b.riseCount = 0
		b.fallCount++
		if b.fallCount >= b.fall {
			b.Up = false
		}
	}
}

// Periodically check the status of this backend
// TODO: ErrLim, Rise and Fall
func (b *Backend) healthCheck() {
	for {
		select {
		case <-b.stopCheck:
			return
		case <-time.After(b.checkInterval):
			b.check()
		}
	}
}

func (b *Backend) Proxy(conn net.Conn) {
	// Backend is a pointer receiver so we can get the address of the fields,
	// but all updates will be done atomically.
	// We still lock b in case of a config update while starting the Proxy.
	b.Lock()
	addr := b.Addr
	dialTimeout := b.dialTimeout
	// pointer values for atomic updates
	conns := &b.Conns
	active := &b.Active
	errorCount := &b.Errors
	bytesSent := &b.Sent
	bytesRcvd := &b.Rcvd
	b.Unlock()

	c, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		log.Println("error connecting to backend", err)
		conn.Close()
		atomic.AddUint64(errorCount, 1)
		return
	}
	bConn := &timeoutConn{
		Conn:      c,
		rwTimeout: b.rwTimeout,
	}

	// TODO: No way to force shutdown. Do we need it?

	atomic.AddInt64(conns, 1)
	atomic.AddInt64(active, 1)
	defer atomic.AddInt64(active, -1)

	// channels to wait on close event
	backendClosed := make(chan bool, 1)
	clientClosed := make(chan bool, 1)

	go broker(bConn, conn, clientClosed, bytesSent, errorCount)
	go broker(conn, bConn, backendClosed, bytesRcvd, errorCount)

	// wait for one half of the proxy to exit, then trigger a shutdown of the
	// other half by calling CloseRead(). This will break the read loop in the
	// broker and fully close the connection.
	var waitFor chan bool
	select {
	case <-clientClosed:
		if err := bConn.Conn.(*net.TCPConn).CloseRead(); err != nil {
			atomic.AddUint64(errorCount, 1)
			log.Printf("proxy error: %s", err)
		}
		waitFor = backendClosed
	case <-backendClosed:
		if err := bConn.Conn.(*net.TCPConn).CloseRead(); err != nil {
			atomic.AddUint64(errorCount, 1)
			log.Printf("proxy error: %s", err)
		}
		waitFor = clientClosed
	}
	// wait for the other connection to close
	<-waitFor
}

// This does the actual data transfer.
// The broker only closes the Read side on error.
// TODO: inline io.Copy so we can implement an idle timeout, as well as get live write counts
// without the extra wrapper.
func broker(dst, src net.Conn, srcClosed chan bool, written, errors *uint64) {
	w := &countingWriter{dst, written}
	_, err := io.Copy(w, src)
	if err != nil {
		atomic.AddUint64(errors, 1)
		log.Printf("Copy error: %s", err)
	}
	if err := src.Close(); err != nil {
		atomic.AddUint64(errors, 1)
		log.Printf("Close error: %s", err)
	}
	srcClosed <- true
}

// A net.Listener that provides a read/write timeout
type timeoutListener struct {
	net.Listener
	rwTimeout time.Duration
}

type countingWriter struct {
	io.Writer
	count *uint64
}

func (w *countingWriter) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	if err != nil {
		return
	}
	atomic.AddUint64(w.count, uint64(n))
	return n, nil
}

func (l *timeoutListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tc := &timeoutConn{
		Conn:      c,
		rwTimeout: l.rwTimeout,
	}
	return tc, nil
}

// A net.Conn that sets a deadline for every read or write operation.
// This will allow the server to close connections that are broken at the
// network level.
type timeoutConn struct {
	net.Conn
	rwTimeout time.Duration
}

func (c *timeoutConn) Read(b []byte) (int, error) {
	if c.rwTimeout > 0 {
		err := c.Conn.SetReadDeadline(time.Now().Add(c.rwTimeout))
		if err != nil {
			return 0, err
		}
	}
	return c.Conn.Read(b)
}

func (c *timeoutConn) Write(b []byte) (int, error) {
	if c.rwTimeout > 0 {
		err := c.Conn.SetWriteDeadline(time.Now().Add(c.rwTimeout))
		if err != nil {
			return 0, err
		}
	}
	return c.Conn.Write(b)
}

func newTimeoutListener(addr string, timeout time.Duration) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	tl := &timeoutListener{
		Listener:  l,
		rwTimeout: timeout,
	}
	return tl, nil
}
