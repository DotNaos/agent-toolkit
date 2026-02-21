package chatcli

import (
	"errors"
	"fmt"
)

func apiError(statusCode int, payload map[string]any) error {
	if payload == nil {
		return fmt.Errorf("request failed with status %d", statusCode)
	}
	if msg, ok := payload["message"].(string); ok && msg != "" {
		return errors.New(msg)
	}
	return fmt.Errorf("request failed with status %d", statusCode)
}
