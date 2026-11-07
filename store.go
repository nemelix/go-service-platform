package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func (s *pgStore) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS visits (
			id    int    PRIMARY KEY,
			count bigint NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("create visits table: %w", err)
	}
	return nil
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
