package odds

import (
	"context"
	"log/slog"

	"learning.local/sportsbook/internal/domain"
)

type Service struct {
	Repo  domain.OddsRepository
	Cache domain.OddsCache
	Log   *slog.Logger
}

func NewService(repo domain.OddsRepository, cache domain.OddsCache, log *slog.Logger) *Service {
	return &Service{
		Repo:  repo,
		Cache: cache,
		Log:   log,
	}
}

func (s *Service) GetOdds(ctx context.Context, marketID string) (*domain.MarketOdds, bool, error) {
	// 1. Try Cache
	if s.Cache != nil {
		m, err := s.Cache.Get(ctx, marketID)
		if err == nil {
			return m, true, nil
		}
	}

	// 2. Try Repo
	m, err := s.Repo.GetByID(ctx, marketID)
	if err != nil {
		return nil, false, err
	}

	// 3. Update Cache (Cache Aside)
	if s.Cache != nil {
		_ = s.Cache.Set(ctx, m)
	}

	return m, false, nil
}

func (s *Service) UpdateOdds(ctx context.Context, odds *domain.MarketOdds) error {
	if err := s.Repo.Save(ctx, odds); err != nil {
		return err
	}

	if s.Cache != nil {
		_ = s.Cache.Set(ctx, odds)
	}

	return nil
}

func (s *Service) ListOdds(ctx context.Context) ([]domain.MarketOdds, error) {
	return s.Repo.ListAll(ctx)
}

func (s *Service) Seed(ctx context.Context) error {
	list, err := s.Repo.ListAll(ctx)
	if err != nil {
		return err
	}
	if len(list) > 0 {
		return nil
	}

	s.Log.Info("seeding initial odds data")
	demo := &domain.MarketOdds{
		MarketID:    "mkt_demo_1",
		OddsVersion: 99,
		Selections: []domain.Selection{
			{SelectionID: "sel_demo_1", Price: 1.95},
		},
		Status: "OPEN",
	}
	return s.UpdateOdds(ctx, demo)
}
