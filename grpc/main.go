package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/franzego/transcoder/grpc/config"
	"github.com/franzego/transcoder/grpc/connection"
	"github.com/franzego/transcoder/grpc/repository"
	pb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transcoder/grpc/service"
	"github.com/franzego/transcoder/grpc/webserver"
	"github.com/franzego/transcoder/grpc/worker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Error loading configuration files")
		os.Exit(1)
	}
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	redisClient, err := connection.NewRedisConnection(ctx, cfg, logger)
	if err != nil {
		logger.Error("Failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	minioClient, err := connection.NewMinioConnection(ctx, &cfg.Minio, logger)
	if err != nil {
		logger.Error("Failed to connect to minio", "error", err)
		os.Exit(1)
	}
	redisRepo := repository.NewRedisRepo(&cfg.Redis, redisClient)
	webClient := webserver.NewWebserverClient(cfg)
	transcoderService := service.NewTranscoderService(
		logger,
		webClient,
		minioClient,
		cfg.Minio.DownloadBucket,
		cfg.FFmpeg.Path,
		cfg.FFmpeg.ProbePath,
	)

	workerPool := worker.NewWorkerPool(cfg.Worker.Count, redisRepo, transcoderService)
	if workerPool == nil {
		slog.Error("failed to initialize worker pool")
		os.Exit(1)
	}
	go workerPool.Run(ctx, cfg.Worker.JobBuffer, cfg.Worker.ResultBuffer)

	gs := grpc.NewServer()
	pb.RegisterTranscoderServiceServer(gs, transcoderService)
	reflection.Register(gs) // need to disable in production

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.Port))
	if err != nil {
		slog.Error("Unable to create listener", "error", err)
		os.Exit(1)
	}

	slog.Info("Starting grpc server", "grpc_port", cfg.Server.Port, "redis_addr", cfg.RedisAddr())
	if err := gs.Serve(l); err != nil {
		slog.Error("gRPC server exited with error", "error", err)
		os.Exit(1)
	}
}
