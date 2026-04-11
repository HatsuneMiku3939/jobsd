package domain

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const (
	DefaultOnFinishCommandTimeoutMS = 5000
	DefaultOnFinishHTTPTimeoutMS    = 3000
	DefaultOnFinishRetryCount       = 1
	DefaultOnFinishRetryBackoffMS   = 1000
	DefaultOnFinishPreviewMaxBytes  = 2048
)

type OnFinishSinkType string

const (
	OnFinishSinkTypeCommand OnFinishSinkType = "command"
	OnFinishSinkTypeHTTP    OnFinishSinkType = "http"
)

func (t OnFinishSinkType) IsValid() bool {
	switch t {
	case OnFinishSinkTypeCommand, OnFinishSinkTypeHTTP:
		return true
	default:
		return false
	}
}

type HookDeliveryStatus string

const (
	HookDeliveryStatusSucceeded HookDeliveryStatus = "succeeded"
	HookDeliveryStatusFailed    HookDeliveryStatus = "failed"
	HookDeliveryStatusTimedOut  HookDeliveryStatus = "timed_out"
)

func (s HookDeliveryStatus) IsValid() bool {
	switch s {
	case HookDeliveryStatusSucceeded, HookDeliveryStatusFailed, HookDeliveryStatusTimedOut:
		return true
	default:
		return false
	}
}

type OnFinishConfig struct {
	Type           OnFinishSinkType   `json:"type"`
	TimeoutMS      int                `json:"timeout_ms,omitempty"`
	RetryCount     int                `json:"retry_count,omitempty"`
	RetryBackoffMS int                `json:"retry_backoff_ms,omitempty"`
	Command        *CommandSinkConfig `json:"command,omitempty"`
	HTTP           *HTTPSinkConfig    `json:"http,omitempty"`
}

type CommandSinkConfig struct {
	Program string   `json:"program"`
	Args    []string `json:"args,omitempty"`
}

type HTTPSinkConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func ParseOnFinishConfigJSON(raw string) (*OnFinishConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("on_finish config must not be empty")
	}

	var config OnFinishConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, fmt.Errorf("parse on_finish config json: %w", err)
	}

	normalized, err := NormalizeOnFinishConfig(config)
	if err != nil {
		return nil, err
	}

	return &normalized, nil
}

func NormalizeOnFinishConfig(config OnFinishConfig) (OnFinishConfig, error) {
	if !config.Type.IsValid() {
		return OnFinishConfig{}, fmt.Errorf("invalid on_finish type %q", config.Type)
	}
	if config.TimeoutMS < 0 {
		return OnFinishConfig{}, fmt.Errorf("timeout_ms must be >= 0")
	}
	if config.RetryCount < 0 {
		return OnFinishConfig{}, fmt.Errorf("retry_count must be >= 0")
	}
	if config.RetryBackoffMS < 0 {
		return OnFinishConfig{}, fmt.Errorf("retry_backoff_ms must be >= 0")
	}

	switch config.Type {
	case OnFinishSinkTypeCommand:
		if config.Command == nil {
			return OnFinishConfig{}, fmt.Errorf("command sink config is required")
		}
		if strings.TrimSpace(config.Command.Program) == "" {
			return OnFinishConfig{}, fmt.Errorf("command.program must not be empty")
		}
		if config.HTTP != nil {
			return OnFinishConfig{}, fmt.Errorf("http sink config must be empty for command type")
		}
		if config.TimeoutMS == 0 {
			config.TimeoutMS = DefaultOnFinishCommandTimeoutMS
		}
	case OnFinishSinkTypeHTTP:
		if config.HTTP == nil {
			return OnFinishConfig{}, fmt.Errorf("http sink config is required")
		}
		if strings.TrimSpace(config.HTTP.URL) == "" {
			return OnFinishConfig{}, fmt.Errorf("http.url must not be empty")
		}
		if err := validateLoopbackURL(config.HTTP.URL); err != nil {
			return OnFinishConfig{}, err
		}
		if config.Command != nil {
			return OnFinishConfig{}, fmt.Errorf("command sink config must be empty for http type")
		}
		if config.TimeoutMS == 0 {
			config.TimeoutMS = DefaultOnFinishHTTPTimeoutMS
		}
	}

	if config.RetryCount == 0 {
		config.RetryCount = DefaultOnFinishRetryCount
	}
	if config.RetryBackoffMS == 0 {
		config.RetryBackoffMS = DefaultOnFinishRetryBackoffMS
	}

	return config, nil
}

func MarshalOnFinishConfigJSON(config *OnFinishConfig) (*string, error) {
	if config == nil {
		return nil, nil
	}

	normalized, err := NormalizeOnFinishConfig(*config)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal on_finish config json: %w", err)
	}

	value := string(payload)
	return &value, nil
}

func validateLoopbackURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse http.url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("http.url must use http or https")
	}

	host := parsed.Hostname()
	switch host {
	case "127.0.0.1", "localhost":
		return nil
	default:
		return fmt.Errorf("http.url host %q must be loopback", host)
	}
}
