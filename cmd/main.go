package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"reflect"

	"github.com/franzego/transgoder/internal/config"
	"github.com/franzego/transgoder/internal/connection"
	"github.com/franzego/transgoder/internal/handler"
	"github.com/franzego/transgoder/internal/repository"
	"github.com/franzego/transgoder/internal/service"
	"github.com/franzego/transgoder/pkg"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/franzego/transgoder/docs"
)

// @title Transcoder API
// @version 0.1.0
// @description API for transcoding workflows with multipart upload support.
// @host localhost:8084
// @basePath /
// @schemes http https
// @consumes application/json
// @produces application/json
func main() {
	ctx := context.Background()
	slog.Info("starting program")

	err := godotenv.Load(".env")
	if err != nil {
		slog.Error("failed to load .env", "error", err)
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize logger
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
	// Connect to PostgreSQL
	postgresConn, err := connection.NewPostgresConnection(ctx, cfg, logger)
	if err != nil {
		slog.Error("Failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer postgresConn.Close()

	// Connect to MinIO
	minioClient, err := connection.NewMinioConnection(ctx, &cfg.Minio, logger)
	if err != nil {
		logger.Error("Failed to connect to minio", "error", err)
		os.Exit(1)
	}
	// Connect to Redis
	redisClient, err := connection.NewRedisConnection(ctx, cfg, logger)
	if err != nil {
		logger.Error("Failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	// Initialize Repository
	repo := repository.NewRepo(postgresConn)
	if repo == nil {
		logger.Error("Failed to initialize repository")
		os.Exit(1)
	}

	// Initialize Services
	repoService := service.NewRepoService(repo)
	minioService := service.NewMinioService(&cfg.Minio, minioClient)
	redisService := service.NewRedisService(repository.NewRedisRepo(&cfg.Redis, redisClient))

	// Initialize Handler
	h := handler.NewHandler(minioService, repoService, redisService, logger, validate)

	// Setup Gin router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// Health check endpoint
	// HealthCheckHandler godoc
	// @Summary Health check
	// @Description Check if the API is running
	// @Tags health
	// @Produce json
	// @Success 200 {string} string "ok"
	// @Router /health [get]
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Serve static files
	router.Static("/web", "./web")
	router.GET("/", func(c *gin.Context) {
		c.File("./web/index.html")
	})

	// Upload routes
	uploadGroup := router.Group("/upload")
	{
		uploadGroup.POST("/initiate", h.InitiateMultipartUploadHandler)
		uploadGroup.POST("/complete", h.CompleteMultipartUploadHandler)
	}

	// Status routes
	statusGroup := router.Group("/status")
	{
		statusGroup.POST("/:id/update", h.UpdateStatus)
		statusGroup.GET("/:id/update", h.GetJobStatus)
	}

	jobsGroup := router.Group("/jobs")
	{
		jobsGroup.GET("/:id/source-url", h.GetSourceVideoURL)
		jobsGroup.GET("/:id/output-url", h.GetOutputVideoURL)
	}

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})

	// Start server
	addr := cfg.ServerAddr()
	logger.Info("Server starting", "address", addr)
	if err := router.Run(addr); err != nil {
		logger.Error("Server failed", "error", err)
		log.Fatalf("Server failed: %v", err)
	}
}
