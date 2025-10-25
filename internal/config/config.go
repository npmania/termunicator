package config

import (
	"os"
)

// Config holds the application configuration
type Config struct {
	Mattermost MattermostConfig
}

// MattermostConfig holds Mattermost-specific configuration
type MattermostConfig struct {
	Host  string
	Token string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		Mattermost: MattermostConfig{
			Host:  os.Getenv("MATTERMOST_HOST"),
			Token: os.Getenv("MATTERMOST_TOKEN"),
		},
	}
}

// Validate checks if required configuration is present
func (c *Config) Validate() error {
	if c.Mattermost.Host == "" {
		return ErrMissingMattermostHost
	}
	if c.Mattermost.Token == "" {
		return ErrMissingMattermostToken
	}
	return nil
}