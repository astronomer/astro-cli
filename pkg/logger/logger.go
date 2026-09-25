package logger

import (
	"io"

	"github.com/sirupsen/logrus"
)

var logger = logrus.New()

func Debug(args ...interface{}) {
	logger.Debug(args...)
}

func Debugf(format string, args ...interface{}) {
	logger.Debugf(format, args...)
}

func Fatal(args ...interface{}) {
	logger.Fatal(args...)
}

func Fatalf(format string, args ...interface{}) {
	logger.Fatalf(format, args...)
}

func GetLevel() logrus.Level {
	return logger.GetLevel()
}

func Infof(format string, args ...interface{}) {
	logger.Infof(format, args...)
}

func SetLevel(level logrus.Level) {
	logger.SetLevel(level)
}

func SetOutput(out io.Writer) {
	logger.SetOutput(out)
}

func Warnf(format string, args ...interface{}) {
	logger.Warnf(format, args...)
}

func IsLevelEnabled(level logrus.Level) bool {
	return logger.IsLevelEnabled(level)
}
