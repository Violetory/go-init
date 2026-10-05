package template

import (
	"context"

	repo "example.com/go-init-template/internal/adapters/postgresql/sqlc"
	"github.com/jackc/pgx/v5"
)

type Service interface {
	Template(ctx context.Context) ([]repo.Template, error)
}

type service struct {
	repo repo.Querier
	db   *pgx.Conn
}

func NewService(queries *repo.Queries, db *pgx.Conn) Service {
	return &service{repo: queries, db: db}
}

func (s *service) Template(ctx context.Context) ([]repo.Template, error) {
	return s.repo.Template(ctx)
}
