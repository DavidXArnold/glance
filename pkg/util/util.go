package util

import (
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// SetupLogger sets configuration for the default logger
func SetupLogger() (err error) {
	var (
		lf = strings.ToLower(viper.GetString("output"))
	)

	// Set log format
	switch lf {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	default:
		log.SetFormatter(&log.TextFormatter{
			DisableLevelTruncation: true,
		})
	}
	return nil
}
