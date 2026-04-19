package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {

	slog.Info("Starting grpc server...")
	gs := grpc.NewServer()
	reflection.Register(gs)

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", 9092))
	if err != nil {
		slog.Error("Unable to create listener", "error", err)
		os.Exit(1)
	}

	// listen for requests
	gs.Serve(l)
}
