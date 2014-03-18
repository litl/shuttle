package main

import (
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"
)

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
