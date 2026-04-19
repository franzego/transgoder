package config

import (
	"fmt"
	"os"
	"strconv"
)

type config struct {
	GrpcPort      int
	RedisAddr     string
	RedisPassword string
	RedisDB       int
}

func LoadConfig() config {
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnvInt("REDIS_PORT", 6379)

	return config{
		GrpcPort:      getEnvInt("GRPC_PORT", 9092),
		RedisAddr:     fmt.Sprintf("%s:%d", redisHost, redisPort),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
