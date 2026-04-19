package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/franzego/transcoder/grpc/config"
	pb "github.com/franzego/transcoder/grpc/server"
	"github.com/franzego/transcoder/grpc/service"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg := config.LoadConfig()
	ctx := context.Background()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		slog.Error("Unable to connect to redis", "addr", cfg.RedisAddr, "error", err)
		os.Exit(1)
	}

	gs := grpc.NewServer()
	pb.RegisterTranscoderServiceServer(gs, &service.TranscoderService{Redis: redisClient})
	reflection.Register(gs)

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GrpcPort))
	if err != nil {
		slog.Error("Unable to create listener", "error", err)
		os.Exit(1)
	}

	slog.Info("Starting grpc server", "grpc_port", cfg.GrpcPort, "redis_addr", cfg.RedisAddr)
	if err := gs.Serve(l); err != nil {
		slog.Error("gRPC server exited with error", "error", err)
		os.Exit(1)
	}
}
