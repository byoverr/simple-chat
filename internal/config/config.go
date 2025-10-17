package config

import "github.com/ilyakaznacheev/cleanenv"

type Config struct {
	KafkaBrokers string `env:"KAFKA_BROKERS" env-default:"localhost:9092" env-description:"Kafka brokers list"`
	MongoURI     string `env:"MONGO_URI" env-default:"mongodb://localhost:27017"`
	MongoDB      string `env:"MONGO_DB" env-default:"notifications"`
	RedisAddr    string `env:"REDIS_ADDR" env-default:"localhost:6379"`
	ApiPort      string `env:"API_PORT" env-default:":8080"`
}

func LoadConfig() (*Config, error) {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
