package repository

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewRepo(t *testing.T) {
	t.Run("nil connection returns nil", func(t *testing.T) {
		repo := NewRepo(nil)
		if repo != nil {
			t.Fatalf("expected nil repo, got %#v", repo)
		}
	})

	t.Run("valid connection builds repo", func(t *testing.T) {
		conn := &pgxpool.Pool{}
		repo := NewRepo(conn)
		if repo == nil {
			t.Fatal("expected repo, got nil")
		}
		if repo.Conn != conn {
			t.Fatalf("expected conn pointer to match, got %p want %p", repo.Conn, conn)
		}
		if repo.Q == nil {
			t.Fatal("expected sqlc queries to be initialized")
		}
	})
}

