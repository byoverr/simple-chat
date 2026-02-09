package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

const (
	levelDebug = "debug"
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
)

type Config struct {
	Env      string
	LogLevel string
	Service  string
}

// New returns configured zerolog.Logger and also sets global logger.
func New(c Config) zerolog.Logger {

	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	zerolog.TimeFieldFormat = time.RFC3339Nano

	level := zerolog.InfoLevel
	if c.LogLevel != "" {
		if l, err := zerolog.ParseLevel(strings.ToLower(c.LogLevel)); err == nil {
			level = l
		}
	}
	zerolog.SetGlobalLevel(level)

	var w io.Writer = os.Stdout
	if strings.EqualFold(c.Env, "local") {
		w = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05.000",
			NoColor:    false,
		}
	}

	l := zerolog.New(w).
		With().
		Timestamp().
		Str("service", c.Service).
		Logger()

	zlog.Logger = l

	return l
}
