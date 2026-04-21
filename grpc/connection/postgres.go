package connection

import (
	"context"
	"log/slog"

	"github.com/franzego/transcoder/grpc/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresConnection(ctx context.Context, c *config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	logger.Info("Starting connection to postgres container")
	conn, err := pgxpool.New(ctx, c.PostgresDSN())
	if err != nil {
		logger.Error("Failed to create postgres pool", "error", err)
		return nil, err
	}

	if err := conn.Ping(ctx); err != nil {
		logger.Error("Failed to ping postgres", "error", err)
		return nil, err
	}

	logger.Info("Successfully connected to postgres")
	return conn, nil
}
