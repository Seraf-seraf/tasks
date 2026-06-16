package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strconv"
	"strings"
	"time"
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	const methodCtx = "config.Duration.UnmarshalYAML"
	if value == nil || strings.TrimSpace(value.Value) == "" {
		return nil
	}
	parsed, err := time.ParseDuration(value.Value)
	if err == nil {
		*d = Duration(parsed)
		return nil
	}
	n, convErr := strconv.ParseInt(value.Value, 10, 64)
	if convErr != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	*d = Duration(time.Duration(n))
	return nil
}

func (d Duration) Std() time.Duration {
	return time.Duration(d)
}

type Config struct {
	HTTP struct {
		Addr string `yaml:"addr"`
	} `yaml:"http"`
	MySQL struct {
		DSN             string   `yaml:"dsn"`
		MaxOpenConns    int      `yaml:"max_open_conns"`
		MaxIdleConns    int      `yaml:"max_idle_conns"`
		ConnMaxLifetime Duration `yaml:"conn_max_lifetime"`
	} `yaml:"mysql"`
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"redis"`
	JWT struct {
		Secret string   `yaml:"secret"`
		TTL    Duration `yaml:"ttl"`
	} `yaml:"jwt"`
	Email struct {
		Endpoint string `yaml:"endpoint"`
	} `yaml:"email"`
}

func Load(path string) (Config, error) {
	const methodCtx = "config.Load"
	c := Config{}
	c.HTTP.Addr = ":8080"
	c.MySQL.MaxOpenConns = 20
	c.MySQL.MaxIdleConns = 10
	c.MySQL.ConnMaxLifetime = Duration(time.Hour)
	c.JWT.Secret = "dev-secret"
	c.JWT.TTL = Duration(24 * time.Hour)
	c.Redis.Addr = "localhost:6379"
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return c, fmt.Errorf("%s: %w", methodCtx, err)
		}
		if err := yaml.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("%s: %w", methodCtx, err)
		}
	}
	env(&c)
	return c, nil
}
func env(c *Config) {
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		c.HTTP.Addr = v
	}
	if v := os.Getenv("MYSQL_DSN"); v != "" {
		c.MySQL.DSN = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		c.Redis.Addr = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JWT.Secret = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Redis.DB = n
		}
	}
}
