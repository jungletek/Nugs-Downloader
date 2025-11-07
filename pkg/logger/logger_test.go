package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// TestSuite for logger package
type LoggerTestSuite struct {
	suite.Suite
}

// SetupTest runs before each test
func (suite *LoggerTestSuite) SetupTest() {
	// Reset the global logger for each test
	log = nil
}

// TearDownTest runs after each test
func (suite *LoggerTestSuite) TearDownTest() {
	// Reset the global logger after each test
	log = nil
}

// TestGetLogger_Initialization tests logger initialization
func (suite *LoggerTestSuite) TestGetLogger_Initialization() {
	logger := GetLogger()

	// Verify logger is not nil
	assert.NotNil(suite.T(), logger)

	// Verify it's a logrus logger
	assert.IsType(suite.T(), &logrus.Logger{}, logger)

	// Verify default level is Info
	assert.Equal(suite.T(), logrus.InfoLevel, logger.GetLevel())

	// Verify formatter is JSON
	formatter, ok := logger.Formatter.(*logrus.JSONFormatter)
	assert.True(suite.T(), ok)
	assert.Equal(suite.T(), "2006-01-02 15:04:05", formatter.TimestampFormat)
}

// TestGetLogger_Singleton tests that GetLogger returns the same instance
func (suite *LoggerTestSuite) TestGetLogger_Singleton() {
	logger1 := GetLogger()
	logger2 := GetLogger()

	// Should return the same instance
	assert.Equal(suite.T(), logger1, logger2)
}

// TestGetLogger_Output tests logger output configuration
func (suite *LoggerTestSuite) TestGetLogger_Output() {
	logger := GetLogger()

	// Verify output is set to stdout (can't easily test stdout directly,
	// but we can verify it's not nil)
	assert.NotNil(suite.T(), logger.Out)
}

// TestWrapError_NilError tests wrapping nil error
func (suite *LoggerTestSuite) TestWrapError_NilError() {
	result := WrapError(nil, map[string]interface{}{"key": "value"})

	assert.Nil(suite.T(), result)
}

// TestWrapError_WithContext tests wrapping error with context
func (suite *LoggerTestSuite) TestWrapError_WithContext() {
	// Capture log output
	var buf bytes.Buffer
	originalLogger := GetLogger()
	testLogger := logrus.New()
	testLogger.SetOutput(&buf)
	testLogger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})
	testLogger.SetLevel(logrus.InfoLevel)

	// Temporarily replace the global logger
	log = testLogger

	defer func() {
		// Restore original logger
		log = originalLogger
	}()

	originalErr := assert.AnError // Use testify's test error
	context := map[string]interface{}{
		"url":    "https://api.example.com",
		"method": "GET",
		"retry":  3,
	}

	result := WrapError(originalErr, context)

	// Verify the returned error is the same
	assert.Equal(suite.T(), originalErr, result)

	// Verify log output contains expected fields
	logOutput := buf.String()
	assert.Contains(suite.T(), logOutput, "Operation failed")

	// Parse JSON log entry
	var logEntry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	lastLine := lines[len(lines)-1]

	err := json.Unmarshal([]byte(lastLine), &logEntry)
	assert.NoError(suite.T(), err)

	// Verify error field is present
	assert.Contains(suite.T(), logEntry, "error")

	// Verify context fields are present
	assert.Equal(suite.T(), "https://api.example.com", logEntry["url"])
	assert.Equal(suite.T(), "GET", logEntry["method"])
	assert.Equal(suite.T(), float64(3), logEntry["retry"]) // JSON numbers are float64
}

// TestWrapError_EmptyContext tests wrapping error with empty context
func (suite *LoggerTestSuite) TestWrapError_EmptyContext() {
	var buf bytes.Buffer
	originalLogger := GetLogger()
	testLogger := logrus.New()
	testLogger.SetOutput(&buf)
	testLogger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})
	testLogger.SetLevel(logrus.InfoLevel)

	log = testLogger
	defer func() { log = originalLogger }()

	originalErr := assert.AnError
	result := WrapError(originalErr, map[string]interface{}{})

	assert.Equal(suite.T(), originalErr, result)

	logOutput := buf.String()
	assert.Contains(suite.T(), logOutput, "Operation failed")
}

// TestWrapError_SpecialCharacters tests wrapping error with special characters in context
func (suite *LoggerTestSuite) TestWrapError_SpecialCharacters() {
	var buf bytes.Buffer
	originalLogger := GetLogger()
	testLogger := logrus.New()
	testLogger.SetOutput(&buf)
	testLogger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})
	testLogger.SetLevel(logrus.InfoLevel)

	log = testLogger
	defer func() { log = originalLogger }()

	originalErr := assert.AnError
	context := map[string]interface{}{
		"special": "chars & symbols <>\"'",
		"unicode": "测试",
	}

	result := WrapError(originalErr, context)
	assert.Equal(suite.T(), originalErr, result)

	logOutput := buf.String()
	assert.Contains(suite.T(), logOutput, "Operation failed")

	// Parse JSON to verify special characters are properly handled
	var logEntry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	lastLine := lines[len(lines)-1]

	err := json.Unmarshal([]byte(lastLine), &logEntry)
	assert.NoError(suite.T(), err)

	// Verify the special characters are preserved in the parsed JSON
	assert.Equal(suite.T(), "chars & symbols <>\"'", logEntry["special"])
	assert.Equal(suite.T(), "测试", logEntry["unicode"])
}

// TestWrapError_LogLevel tests that error is logged at Error level
func (suite *LoggerTestSuite) TestWrapError_LogLevel() {
	var buf bytes.Buffer
	originalLogger := GetLogger()
	testLogger := logrus.New()
	testLogger.SetOutput(&buf)
	testLogger.SetFormatter(&logrus.TextFormatter{}) // Use text formatter for easier parsing
	testLogger.SetLevel(logrus.InfoLevel)

	log = testLogger
	defer func() { log = originalLogger }()

	originalErr := assert.AnError
	context := map[string]interface{}{"test": "value"}

	WrapError(originalErr, context)

	logOutput := buf.String()
	// Should contain "error" level indicator
	assert.Contains(suite.T(), logOutput, "error")
	assert.Contains(suite.T(), logOutput, "Operation failed")
}

// TestLogger_ConcurrentAccess tests concurrent access to GetLogger
func (suite *LoggerTestSuite) TestLogger_ConcurrentAccess() {
	// Reset logger
	log = nil

	done := make(chan bool, 10)

	// Launch multiple goroutines to access GetLogger concurrently
	for i := 0; i < 10; i++ {
		go func() {
			logger := GetLogger()
			assert.NotNil(suite.T(), logger)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify logger is still accessible
	logger := GetLogger()
	assert.NotNil(suite.T(), logger)
	assert.Equal(suite.T(), logrus.InfoLevel, logger.GetLevel())
}

// TestLogger_CustomFormatter tests that the logger uses the expected JSON formatter
func (suite *LoggerTestSuite) TestLogger_CustomFormatter() {
	logger := GetLogger()

	formatter := logger.Formatter
	jsonFormatter, ok := formatter.(*logrus.JSONFormatter)

	assert.True(suite.T(), ok, "Logger should use JSONFormatter")
	assert.NotNil(suite.T(), jsonFormatter)
	assert.Equal(suite.T(), "2006-01-02 15:04:05", jsonFormatter.TimestampFormat)
}

// Run the test suite
func TestLoggerTestSuite(t *testing.T) {
	suite.Run(t, new(LoggerTestSuite))
}
