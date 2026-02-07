package framework

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

func SetupLogger(stateDir string) (io.Closer, error) {
	logPath := filepath.Join(stateDir, "status.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	// MultiWriter to both stdout and the file
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)

	return f, nil
}
