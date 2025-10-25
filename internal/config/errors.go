package config

import "errors"

var (
	ErrMissingMattermostHost  = errors.New("MATTERMOST_HOST environment variable is required")
	ErrMissingMattermostToken = errors.New("MATTERMOST_TOKEN environment variable is required")
)