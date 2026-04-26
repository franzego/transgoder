package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Server    GrpcServerConfig
	Worker    WorkerConfig
	Postgres  PostgresConfig
	Redis     RedisConfig
	Minio     MinioConfig
	JWT       JWTConfig
	FFmpeg    FFmpegConfig
	WebServer WebServerConfig
	// Logger   LoggerConfig
}

type WebServerConfig struct {
	ServerUrl string
}

type GrpcServerConfig struct {
	Host string
	Port int
}

type WorkerConfig struct {
	Count        int
	JobBuffer    int
	ResultBuffer int
}

type PostgresConfig struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
	SSLMode  string
}

type RedisConfig struct {
	Host       string
	Port       int
	Password   string
	DB         int
	StreamName string
	GroupName  string
}

type MinioConfig struct {
	Endpoint       string
	AccessKey      string
	SecretKey      string
	UseSSL         bool
	UploadBucket   string
	DownloadBucket string
}

type JWTConfig struct {
	Secret     string
	Issuer     string
	TTLMinutes int
}

type FFmpegConfig struct {
	Path      string
	ProbePath string
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: GrpcServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8099),
		},
		Worker: WorkerConfig{
			Count:        getEnvInt("WORKER_COUNT", 4),
			JobBuffer:    getEnvInt("WORKER_JOB_BUFFER", 100),
			ResultBuffer: getEnvInt("WORKER_RESULT_BUFFER", 100),
		},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnvInt("POSTGRES_PORT", 5432), // remember to remove defaults
			DB:       getEnv("POSTGRES_DB", "transcoder"),
			User:     getEnv("POSTGRES_USER", "transcoder"),
			Password: getEnv("POSTGRES_PASSWORD", ""),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Host:       getEnv("REDIS_HOST", "localhost"),
			Port:       getEnvInt("REDIS_PORT", 6379),
			Password:   getEnv("REDIS_PASSWORD", ""),
			DB:         getEnvInt("REDIS_DB", 0),
			StreamName: getEnv("REDIS_STREAM_NAME", "transcoder_stream"),
			GroupName:  getEnv("REDIS_GROUP_NAME", "transcoder_group"),
		},
		Minio: MinioConfig{
			Endpoint:       getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey:      getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey:      getEnv("MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:         getEnvBool("MINIO_USE_SSL", false),
			UploadBucket:   getEnv("MINIO_UPLOAD_BUCKET", "uploads"),
			DownloadBucket: getEnv("MINIO_DOWNLOAD_BUCKET", "downloads"),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", "change_me"),
			Issuer:     getEnv("JWT_ISSUER", "transcoder"),
			TTLMinutes: getEnvInt("JWT_TTL_MINUTES", 60),
		},
			FFmpeg: FFmpegConfig{
				Path:      getEnv("FFMPEG_PATH", "/usr/bin/ffmpeg"),
				ProbePath: getEnv("FFPROBE_PATH", "/usr/bin/ffprobe"),
			},
		WebServer: WebServerConfig{
			ServerUrl: getEnv("WEB_SERVER_URL", "http://localhost:8084"),
		},
		// Logger: LoggerConfig{
		// 	Level: getEnv("LOG_LEVEL", "info"),
		// },
	}

	return cfg, nil
}

func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port)
}

func (c *Config) PostgresDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Postgres.Host,
		c.Postgres.Port,
		c.Postgres.User,
		c.Postgres.Password,
		c.Postgres.DB,
		c.Postgres.SSLMode,
	)
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

func getEnvBool(key string, fallback bool) bool {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
