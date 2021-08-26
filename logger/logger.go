package logger

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var once sync.Once
var sugarLogger *zap.SugaredLogger
var atom = zap.NewAtomicLevelAt(zap.InfoLevel)

func NewLogger() *zap.SugaredLogger {
	once.Do(func() {
		encoderCfg := zap.NewDevelopmentEncoderConfig()
		newLogger := zap.New(zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderCfg),
			zapcore.Lock(os.Stdout),
			atom,
		))

		defer newLogger.Sync()
		sugarLogger = newLogger.Sugar()

	})

	return sugarLogger
}

func SetLevelInfo() {
	atom.SetLevel(zap.InfoLevel)
}

func SetLevelDebug() {
	atom.SetLevel(zap.DebugLevel)
}

func SetLevelError() {
	atom.SetLevel(zap.ErrorLevel)
}
