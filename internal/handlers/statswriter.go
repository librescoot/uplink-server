package handlers

import (
	"bufio"
	"net"
	"net/http"
)

// StatsResponseWriter wraps http.ResponseWriter to inject StatsConn during hijacking
type StatsResponseWriter struct {
	http.ResponseWriter
	statsConn *StatsConn
}

// NewStatsResponseWriter creates a new stats-tracking response writer
func NewStatsResponseWriter(w http.ResponseWriter) *StatsResponseWriter {
	return &StatsResponseWriter{ResponseWriter: w}
}

// Hijack wraps the underlying Hijack to inject StatsConn
func (srw *StatsResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := srw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}

	// Wrap the connection with stats tracking
	srw.statsConn = NewStatsConn(conn)
	return srw.statsConn, rw, nil
}

// GetStatsConn returns the stats conn (may be nil if Hijack hasn't been called)
func (srw *StatsResponseWriter) GetStatsConn() *StatsConn {
	return srw.statsConn
}
