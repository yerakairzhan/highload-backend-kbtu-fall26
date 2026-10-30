package log

import (
	"os"

	"github.com/sirupsen/logrus"
)

func InitLogger(levelStr string) {
	logrus.SetOutput(os.Stdout)
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		logrus.Errorf("Failed to parse log level, defaulting to INFO: %v", err)
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)
}
