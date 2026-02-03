package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	NATS  NATSConfig
	Redis RedisConfig
	App   AppConfig
}

type NATSConfig struct {
	URL           string
	Streams       StreamConfig
	MaxReconnects int
}

type StreamConfig struct {
	CommandSubject     string
	InteractionSubject string
	OutputSubject      string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

type AppConfig struct {
	HeartbeatMs int
	WorkerID    string
	LogLevel    string
	Port        string
}

func Load() (*Config, error) {
	viper.SetDefault("nats.url", "nats://localhost:4222")
	viper.SetDefault("nats.streams.commands", "archon.commands")
	viper.SetDefault("nats.streams.interactions", "archon.interactions")
	viper.SetDefault("nats.streams.outputs", "archon.outputs")
	viper.SetDefault("nats.max_reconnects", 10)

	viper.SetDefault("redis.addr", "localhost:6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.pool_size", 10)

	viper.SetDefault("app.heartbeat_ms", 100)
	viper.SetDefault("app.worker_id", "worker-1")
	viper.SetDefault("app.log_level", "info")
	viper.SetDefault("app.port", "8080")
	viper.SetEnvPrefix("ARCHON")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	_ = viper.BindEnv("nats.url")
	_ = viper.BindEnv("nats.max_reconnects")
	_ = viper.BindEnv("redis.addr")
	_ = viper.BindEnv("redis.password")
	_ = viper.BindEnv("redis.db")
	_ = viper.BindEnv("redis.pool_size")
	_ = viper.BindEnv("app.heartbeat_ms")
	_ = viper.BindEnv("app.worker_id")
	_ = viper.BindEnv("app.log_level")
	_ = viper.BindEnv("app.port")

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	println(cfg.App.WorkerID)

	return &cfg, nil
}
