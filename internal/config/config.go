package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret string
	Issuer string
	Expiry time.Duration
}

type Config struct {
	MySQL MySQLConfig
	Redis RedisConfig
	JWT   JWTConfig
}

func Default() *Config {
	return &Config{
		MySQL: MySQLConfig{
			Host:     "127.0.0.1",
			Port:     3306,
			User:     "root",
			Password: "",
			Database: "im",
		},
		Redis: RedisConfig{
			Addr:     "127.0.0.1:6379",
			Password: "",
			DB:       0,
		},
		JWT: JWTConfig{
			Secret: "change-me",
			Issuer: "im-system",
			Expiry: 24 * time.Hour,
		},
	}
}

func (c MySQLConfig) DSN() string {
	user := c.User
	password := c.Password
	if strings.ContainsAny(user, ":@") {
		user = quoteDSNValue(user)
	}
	if strings.ContainsAny(password, ":@") {
		password = quoteDSNValue(password)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&loc=Local",
		user,
		password,
		c.Host,
		c.Port,
		c.Database,
	)
}

func quoteDSNValue(value string) string {
	replacer := strings.NewReplacer("@", "%40", ":", "%3A")
	return replacer.Replace(value)
}

func Load(path string) (*Config, error) {
	cfg := Default()

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if cfg.JWT.Secret == "" {
				return nil, errors.New("jwt secret is required")
			}
			return cfg, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	section := ""
	redisHost := ""
	redisPort := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch section {
		case "mysql":
			applyMySQLValue(&cfg.MySQL, key, value)
		case "redis":
			switch key {
			case "addr":
				cfg.Redis.Addr = value
			case "password":
				cfg.Redis.Password = value
			case "db":
				if parsed, err := strconv.Atoi(value); err == nil {
					cfg.Redis.DB = parsed
				}
			case "host":
				redisHost = value
			case "port":
				if parsed, err := strconv.Atoi(value); err == nil {
					redisPort = parsed
				}
			}
		case "jwt":
			switch key {
			case "secret":
				cfg.JWT.Secret = value
			case "issuer":
				if value != "" {
					cfg.JWT.Issuer = value
				}
			case "expiry":
				if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
					cfg.JWT.Expiry = time.Duration(parsed) * time.Second
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if cfg.Redis.Addr == "" {
		host := redisHost
		if host == "" {
			host = "127.0.0.1"
		}
		port := redisPort
		if port == 0 {
			port = 6379
		}
		cfg.Redis.Addr = fmt.Sprintf("%s:%d", host, port)
	}

	if cfg.JWT.Secret == "" {
		return nil, errors.New("jwt secret is required")
	}

	return cfg, nil
}

func applyMySQLValue(mysql *MySQLConfig, key, value string) {
	switch key {
	case "host":
		if value != "" {
			mysql.Host = value
		}
	case "port":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			mysql.Port = parsed
		}
	case "user":
		if value != "" {
			mysql.User = value
		}
	case "password":
		mysql.Password = value
	case "database":
		if value != "" {
			mysql.Database = value
		}
	}
}
