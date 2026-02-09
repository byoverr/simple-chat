package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	App  App
	GRPC GRPC
	DB   DB
	Auth Auth
	Obs  Observability
}

type App struct {
	Env      string `env:"APP_ENV" env-default:"local"`
	LogLevel string `env:"LOG_LEVEL" env-default:"info"`
}

type GRPC struct {
	Addr string `env:"GRPC_ADDR" env-default:":50051"`
}

type DB struct {
	PostgresURL string `env:"DATABASE_URL" env-required:"true"`
}

type Auth struct {
	JWT     JWT
	Refresh Refresh
	Pass    Password
}

type JWT struct {
	AccessTTL time.Duration `env:"JWT_ACCESS_TTL" env-default:"10m"`
	Secret    string        `env:"JWT_SECRET" env-required:"true"`
}

type Refresh struct {
	TTL         time.Duration `env:"REFRESH_TTL" env-default:"720h"`
	Pepper      string        `env:"REFRESH_PEPPER" env-required:"true"`
	TokenBytes  int           `env:"REFRESH_TOKEN_BYTES" env-default:"32"`
	ReuseDetect bool          `env:"REFRESH_REUSE_DETECT" env-default:"true"`
}

type Password struct {
	Algo       string `env:"PASSWORD_HASH_ALGO" env-default:"bcrypt"`
	BcryptCost int    `env:"BCRYPT_COST" env-default:"12"`
	MinLen     int    `env:"PASSWORD_MIN_LEN" env-default:"8"`
}

type Observability struct {
	MetricsAddr  string `env:"METRICS_ADDR" env-default:":9090"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	ServiceName  string `env:"OTEL_SERVICE_NAME" env-default:"auth-service"`
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

	if c.Auth.JWT.AccessTTL <= 0 {
		errs = append(errs, fmt.Errorf("JWT_ACCESS_TTL must be > 0, got %s", c.Auth.JWT.AccessTTL))
	}
	if len(c.Auth.JWT.Secret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET too short: recommend 32+ chars (better 64+)"))
	}
	if c.Auth.Refresh.TTL <= 0 {
		errs = append(errs, fmt.Errorf("REFRESH_TTL must be > 0, got %s", c.Auth.Refresh.TTL))
	}
	if c.Auth.Refresh.TokenBytes < 16 {
		errs = append(errs, fmt.Errorf("REFRESH_TOKEN_BYTES too small: %d (recommend 32+)", c.Auth.Refresh.TokenBytes))
	}
	if c.Auth.Pass.MinLen < 6 {
		errs = append(errs, fmt.Errorf("PASSWORD_MIN_LEN too small: %d", c.Auth.Pass.MinLen))
	}
	if c.Auth.Pass.Algo == "bcrypt" && (c.Auth.Pass.BcryptCost < 10 || c.Auth.Pass.BcryptCost > 15) {
		errs = append(errs, fmt.Errorf("BCRYPT_COST unusual: %d (recommend 10-12)", c.Auth.Pass.BcryptCost))
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
