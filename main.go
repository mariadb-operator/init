package main

import (
	"flag"
	"log"

	"github.com/mariadb-operator/agent/pkg/logger"
)

var (
	configDir string
	stateDir  string

	logLevel       string
	logTimeEncoder string
	logDev         bool
)

func main() {
	flag.StringVar(&configDir, "config-dir", "/etc/mysql/mariadb.conf.d", "The directory that contains MariaDB configuration files")
	flag.StringVar(&stateDir, "state-dir", "/var/lib/mysql", "The directory that contains MariaDB state files")

	flag.StringVar(&logLevel, "log-level", "info", "Log level to use, one of: "+
		"debug, info, warn, error, dpanic, panic, fatal.")
	flag.StringVar(&logTimeEncoder, "log-time-encoder", "epoch", "Log time encoder to use, one of: "+
		"epoch, millis, nano, iso8601, rfc3339 or rfc3339nano")
	flag.BoolVar(&logDev, "log-dev", false, "Enable development logs")

	flag.Parse()

	logger, err := logger.NewLogger(
		logger.WithLogLevel(logLevel),
		logger.WithTimeEncoder(logTimeEncoder),
		logger.WithDevelopment(logDev),
	)
	if err != nil {
		log.Fatalf("error creating logger: %v", err)
	}

	logger.Info("Staring MariaDB init")
}
