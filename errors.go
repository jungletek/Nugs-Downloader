package main

import "fmt"

// NetworkError represents network-related errors
type NetworkError struct {
	URL     string
	Status  int
	Message string
}

func (e NetworkError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("network error: %s (HTTP %d)", e.Message, e.Status)
	}
	return fmt.Sprintf("network error: %s", e.Message)
}

// APIError represents API-related errors
type APIError struct {
	Endpoint string
	Code     int
	Message  string
}

func (e APIError) Error() string {
	return fmt.Sprintf("API error at %s: %s (code: %d)", e.Endpoint, e.Message, e.Code)
}

// FileError represents file system errors
type FileError struct {
	Path    string
	Op      string
	Message string
}

func (e FileError) Error() string {
	return fmt.Sprintf("file error: %s on %s: %s", e.Op, e.Path, e.Message)
}

// ConfigError represents configuration errors
type ConfigError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e ConfigError) Error() string {
	return fmt.Sprintf("config error: %s (field: %s, value: %v)", e.Message, e.Field, e.Value)
}

// WrapError adds context to an error and logs it
func WrapError(err error, context map[string]interface{}) error {
	if err == nil {
		return nil
	}

	// Log the error with context
	logEntry := GetLogger().WithError(err)
	for k, v := range context {
		logEntry = logEntry.WithField(k, v)
	}
	logEntry.Error("Operation failed")

	return fmt.Errorf("%w (context: %v)", err, context)
}
