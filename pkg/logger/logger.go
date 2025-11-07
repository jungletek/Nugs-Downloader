package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

var log *logrus.Logger

// GetLogger returns the global logger instance
func GetLogger() *logrus.Logger {
	if log == nil {
		log = logrus.New()
		log.SetOutput(os.Stdout)
		log.SetLevel(logrus.InfoLevel)

		// JSON format for better parsing
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}
	return log
}

// WrapError wraps an error with additional context
func WrapError(err error, context map[string]interface{}) error {
	if err == nil {
		return nil
	}

	fields := logrus.Fields{}
	for k, v := range context {
		fields[k] = v
	}

	// Log the error with context
	GetLogger().WithFields(fields).WithError(err).Error("Operation failed")

	return err
}
