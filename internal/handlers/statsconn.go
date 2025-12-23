package handlers

import (
	"net"
	"sync/atomic"
)

// StatsConn wraps a net.Conn to track wire-level bytes sent/received
type StatsConn struct {
	net.Conn
	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
}

// NewStatsConn creates a new stats-tracking connection wrapper
func NewStatsConn(conn net.Conn) *StatsConn {
	return &StatsConn{Conn: conn}
}

// Read wraps the underlying Read to count bytes
func (sc *StatsConn) Read(b []byte) (n int, err error) {
	n, err = sc.Conn.Read(b)
	if n > 0 {
		sc.bytesRead.Add(int64(n))
	}
	return n, err
}

// Write wraps the underlying Write to count bytes
func (sc *StatsConn) Write(b []byte) (n int, err error) {
	n, err = sc.Conn.Write(b)
	if n > 0 {
		sc.bytesWritten.Add(int64(n))
	}
	return n, err
}

// BytesRead returns total bytes read from the wire (including compression overhead)
func (sc *StatsConn) BytesRead() int64 {
	return sc.bytesRead.Load()
}

// BytesWritten returns total bytes written to the wire (including compression overhead)
func (sc *StatsConn) BytesWritten() int64 {
	return sc.bytesWritten.Load()
}
