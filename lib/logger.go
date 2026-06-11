package lib

import (
	"strings"

	waLog "go.mau.fi/whatsmeow/util/log"
)

type FilterLogger struct {
	waLog.Logger
}

func (l *FilterLogger) Errorf(msg string, args ...interface{}) {
	if strings.Contains(msg, "Failed to handle retry receipt") {
		return
	}
	if strings.Contains(msg, "EOF") || strings.Contains(msg, "failed to read frame header") {
		return
	}
	l.Logger.Errorf(msg, args...)
}

func (l *FilterLogger) Warnf(msg string, args ...interface{}) {
	if strings.Contains(msg, "mismatching MAC") || strings.Contains(msg, "Failed to decrypt") {
		return
	}
	l.Logger.Warnf(msg, args...)
}

func (l *FilterLogger) Sub(module string) waLog.Logger {
	return &FilterLogger{Logger: l.Logger.Sub(module)}
}
