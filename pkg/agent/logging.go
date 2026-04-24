package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// maxLogSize caps the agent log file. When exceeded, the current file is
// rolled over to a sibling `.old` before a fresh one is opened — so at most
// ~2x this value is kept on disk.
const maxLogSize = 1 << 20 // 1 MiB

func logFilePath() string {
	return filepath.Join(config.HomeConfigPath, "otto", "logs", "agent.log")
}

// redirectCLILogs routes the CLI's own log output to a file so background
// writers don't stomp on Otto's TUI. The returned closer should be deferred.
// Otto is unaffected — it writes through its own pino logger.
func redirectCLILogs() (io.Closer, error) {
	path := logFilePath()
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return nil, fmt.Errorf("creating log dir: %w", err)
	}
	rotateIfLarge(path, maxLogSize)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFilePerm)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	logger.SetOutput(f)
	// Bump level now that logs go to a file, so update hints and debug lines
	// are captured instead of being dropped by the default warn threshold.
	if logger.GetLevel() < logrus.InfoLevel {
		logger.SetLevel(logrus.InfoLevel)
	}
	return f, nil
}

// rotateIfLarge renames path → path.old when path exceeds maxBytes. Best-effort;
// any failure is silently ignored (the logger just keeps appending).
func rotateIfLarge(path string, maxBytes int64) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxBytes {
		return
	}
	_ = os.Rename(path, path+".old")
}
