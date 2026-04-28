package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/franzego/transcoder/internal/config"
	"github.com/franzego/transcoder/internal/connection"
	"github.com/franzego/transcoder/internal/grpcclient"
	"github.com/franzego/transcoder/internal/handler"
	"github.com/franzego/transcoder/internal/observability"
	"github.com/franzego/transcoder/internal/repository"
	"github.com/franzego/transcoder/internal/service"
	"github.com/franzego/transcoder/pkg"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/franzego/transcoder/docs"
)

// @title Transcoder API
// @version 0.2.0
// @description API for transcoding workflows with multipart upload support.
// @host localhost:8084
// @basePath /
// @schemes http https
// @consumes application/json
// @produces application/json
func main() {
	ctx := context.Background()
	slog.Info("starting program")

	if err := godotenv.Load(".env"); err != nil {
		slog.Error("failed to load .env", "error", err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger := cfg.Logger.LoadLogger()
	logger.Info("Starting Transcoder API", "port", cfg.Server.Port)

	validate := validator.New()
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := fld.Tag.Get("json")
		if name == "-" {
			return ""
		}
		return name
	})
	pkg.RegisterCustomValidations(validate)

	postgresConn, err := connection.NewPostgresConnection(ctx, cfg, logger)
	if err != nil {
		slog.Error("Failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer postgresConn.Close()

	minioClient, err := connection.NewMinioConnection(ctx, &cfg.Minio, logger)
	if err != nil {
		logger.Error("Failed to connect to minio", "error", err)
		os.Exit(1)
	}

	redisClient, err := connection.NewRedisConnection(ctx, cfg, logger)
	if err != nil {
		logger.Error("Failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	repo := repository.NewRepo(postgresConn)
	if repo == nil {
		logger.Error("Failed to initialize repository")
		os.Exit(1)
	}

	repoService := service.NewRepoService(repo)
	minioService := service.NewMinioService(&cfg.Minio, minioClient)
	redisService := service.NewRedisService(repository.NewRedisRepo(&cfg.Redis, redisClient))
	transcoderClient, err := grpcclient.New(cfg.Grpc.Addr)
	if err != nil {
		logger.Error("Failed to create grpc client", "error", err)
		os.Exit(1)
	}
	defer transcoderClient.Close()

	h := handler.NewHandler(minioService, repoService, redisService, transcoderClient, logger, validate)

	metrics := observability.New()
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.Use(observability.Middleware(metrics))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "transcoder-api"})
	})

	router.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		deps := map[string]string{"postgres": "ok", "redis": "ok", "minio": "ok"}
		if err := postgresConn.Ping(ctx); err != nil {
			deps["postgres"] = err.Error()
		}
		if err := redisClient.Ping(ctx).Err(); err != nil {
			deps["redis"] = err.Error()
		}
		if _, err := minioClient.BucketExists(ctx, cfg.Minio.UploadBucket); err != nil {
			deps["minio"] = err.Error()
		}

		if deps["postgres"] == "ok" && deps["redis"] == "ok" && deps["minio"] == "ok" {
			c.JSON(http.StatusOK, gin.H{"status": "ready", "dependencies": deps})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "dependencies": deps})
	})

	router.GET("/metrics", func(c *gin.Context) {
		queueDepth, err := redisClient.XLen(c.Request.Context(), cfg.Redis.StreamName).Result()
		if err != nil {
			queueDepth = -1
		}
		s := metrics.Snapshot()
		c.JSON(http.StatusOK, gin.H{"metrics": s, "queue_depth": queueDepth})
	})

	router.Static("/web", "./web")
	router.GET("/", func(c *gin.Context) {
		c.File("./web/index.html")
	})

	uploadGroup := router.Group("/upload")
	{
		uploadGroup.POST("/initiate", h.InitiateMultipartUploadHandler)
		uploadGroup.POST("/complete", h.CompleteMultipartUploadHandler)
	}

	statusGroup := router.Group("/status")
	{
		statusGroup.POST("/:id/update", h.UpdateStatus)
		statusGroup.GET("/:id/update", h.GetJobStatus)
	}

	jobsGroup := router.Group("/jobs")
	{
		jobsGroup.POST("", h.CreateJob)
		jobsGroup.GET("/:id/source-url", h.GetSourceVideoURL)
		jobsGroup.GET("/:id/output-url", h.GetOutputVideoURL)
		jobsGroup.GET("/:id/download", h.DownloadOutputVideo)
		jobsGroup.GET("/:id/transcode-profile", h.GetTranscodeProfile)
	}

	presetsGroup := router.Group("/presets")
	{
		presetsGroup.GET("", h.ListPresets)
		presetsGroup.GET("/:id", h.GetPreset)
	}

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})

	addr := cfg.ServerAddr()
	logger.Info("Server starting", "address", addr)
	if err := router.Run(addr); err != nil {
		logger.Error("Server failed", "error", err)
		log.Fatalf("Server failed: %v", err)
	}
}
