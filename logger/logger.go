package logger

import (
	"go.uber.org/zap"
)

func NewLogger() *zap.SugaredLogger {
	var newLogger, _ = zap.NewDevelopment()
	var logger = newLogger.Sugar()
	return logger

}
