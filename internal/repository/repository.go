package repository

import (
	"github.com/franzego/transcoder/internal/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	Q    *sqlc.Queries
	Conn *pgxpool.Pool
}

func NewRepo(conn *pgxpool.Pool) *Repo {
	if conn == nil {
		return nil
	}
	return &Repo{
		Q:    sqlc.New(conn),
		Conn: conn,
	}
}
