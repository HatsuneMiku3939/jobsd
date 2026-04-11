package app

import (
	"encoding/json"
	"fmt"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func parseOnFinishConfigFlag(raw string) (*domain.OnFinishConfig, error) {
	return domain.ParseOnFinishConfigJSON(raw)
}

func parseOptionalOnFinishConfig(raw string) (*domain.OnFinishConfig, error) {
	if raw == "" {
		return nil, nil
	}

	return parseOnFinishConfigFlag(raw)
}

func formatOnFinishConfigValue(config *domain.OnFinishConfig) string {
	if config == nil {
		return ""
	}

	payload, err := json.Marshal(config)
	if err != nil {
		return fmt.Sprintf("<invalid: %v>", err)
	}

	return string(payload)
}

func onFinishConfigsEqual(left *domain.OnFinishConfig, right *domain.OnFinishConfig) bool {
	leftJSON, err := domain.MarshalOnFinishConfigJSON(left)
	if err != nil {
		return false
	}
	rightJSON, err := domain.MarshalOnFinishConfigJSON(right)
	if err != nil {
		return false
	}
	switch {
	case leftJSON == nil && rightJSON == nil:
		return true
	case leftJSON == nil || rightJSON == nil:
		return false
	default:
		return *leftJSON == *rightJSON
	}
}
