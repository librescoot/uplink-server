package models

import "time"

// Config represents the server configuration
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Auth    AuthConfig    `yaml:"auth"`
	Storage StorageConfig `yaml:"storage"`
	Logging LoggingConfig `yaml:"logging"`
}

// ServerConfig contains server settings
type ServerConfig struct {
	WSPort            int    `yaml:"ws_port"`
	LPPort            int    `yaml:"lp_port"`
	SSEPort           int    `yaml:"sse_port"`
	KeepaliveInterval string `yaml:"keepalive_interval"`
	LPTimeout         string `yaml:"lp_timeout"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	APIKey string            `yaml:"api_key"`
	Tokens map[string]string `yaml:"tokens"`
}

// StorageConfig contains storage settings
type StorageConfig struct {
	Type string `yaml:"type"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level         string `yaml:"level"`
	StatsInterval string `yaml:"stats_interval"`
}

// GetKeepaliveInterval parses and returns the keepalive interval
func (c *ServerConfig) GetKeepaliveInterval() time.Duration {
	d, err := time.ParseDuration(c.KeepaliveInterval)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// GetLPTimeout parses and returns the long poll timeout
func (c *ServerConfig) GetLPTimeout() time.Duration {
	d, err := time.ParseDuration(c.LPTimeout)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}

// GetStatsInterval parses and returns the stats interval
func (c *LoggingConfig) GetStatsInterval() time.Duration {
	d, err := time.ParseDuration(c.StatsInterval)
	if err != nil {
		return 30 * time.Second
	}
	return d
}
