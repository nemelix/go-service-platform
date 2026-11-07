package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func (s *pgStore) Visits(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count FROM visits WHERE id = 1`).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

func (s *pgStore) AddVisit(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO visits (id, count) VALUES (1, 1)
		ON CONFLICT (id) DO UPDATE SET count = visits.count + 1
		RETURNING count`).Scan(&n)
	return n, err
}

func (s *pgStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
