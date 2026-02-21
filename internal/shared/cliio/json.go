package cliio

import (
	"encoding/json"
	"os"
)

// OutputJSON writes a single JSON object/array payload to stdout.
func OutputJSON(payload any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
}

// FormatErrorJSON standardizes machine-readable CLI error output.
func FormatErrorJSON(err error) string {
	if err == nil {
		return `{"status":"error","message":"unknown error"}`
	}

	payload, marshalErr := json.Marshal(map[string]string{
		"status":  "error",
		"message": err.Error(),
	})
	if marshalErr != nil {
		return `{"status":"error","message":"failed to marshal error"}`
	}
	return string(payload)
}
