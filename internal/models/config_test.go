package models

import (
	"testing"
	"time"
)

func TestGetKeepaliveInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"30s", 30 * time.Second},
		{"1h", time.Hour},
		{"", 5 * time.Minute},     // default
		{"invalid", 5 * time.Minute}, // default on error
	}

	for _, tt := range tests {
		c := ServerConfig{KeepaliveInterval: tt.input}
		got := c.GetKeepaliveInterval()
		if got != tt.expected {
			t.Errorf("GetKeepaliveInterval(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestGetLPTimeout(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"", 24 * time.Hour},
		{"invalid", 24 * time.Hour},
	}

	for _, tt := range tests {
		c := ServerConfig{LPTimeout: tt.input}
		got := c.GetLPTimeout()
		if got != tt.expected {
			t.Errorf("GetLPTimeout(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestGetIdleTimeout(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"1h", time.Hour},
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		c := ServerConfig{IdleTimeout: tt.input}
		got := c.GetIdleTimeout()
		if got != tt.expected {
			t.Errorf("GetIdleTimeout(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestGetStatsInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30s", 30 * time.Second},
		{"", 30 * time.Second},
		{"invalid", 30 * time.Second},
	}

	for _, tt := range tests {
		c := LoggingConfig{StatsInterval: tt.input}
		got := c.GetStatsInterval()
		if got != tt.expected {
			t.Errorf("GetStatsInterval(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}
