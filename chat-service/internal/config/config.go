package config

import (
	"errors"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	App  App
	GRPC GRPC
	HTTP HTTP
	DB   DB
	Auth Auth
}

type App struct {
	Env      string `env:"APP_ENV" env-default:"local"`
	LogLevel string `env:"LOG_LEVEL" env-default:"info"`
}

type GRPC struct {
	Addr string `env:"GRPC_ADDR" env-default:":50052"`
}

type HTTP struct {
	Addr string `env:"HTTP_ADDR" env-default:":8081"`
}

type DB struct {
	PostgresURL string `env:"DATABASE_URL" env-required:"true"`
}

type Auth struct {
	JWTSecret string `env:"JWT_SECRET" env-required:"true"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, err
	}
	return &cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	var errs []error
	if len(c.Auth.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET too short: recommend 32+ chars"))
	}
	if len(errs) == 0 {
		return nil
	}
	msg := "config validation failed:"
	for _, e := range errs {
		msg += "\n - " + e.Error()
	}
	return errors.New(msg)
}
