package service

import (
	"context"
	"fmt"

	"github.com/Seraf-seraf/tasks/internal/domain"
	repository "github.com/Seraf-seraf/tasks/internal/repository/mysql"
)

type ReportService struct {
	repo *repository.Repo
}

func NewReportService(r *repository.Repo) *ReportService {
	return &ReportService{repo: r}
}

func (s *ReportService) Data(ctx context.Context) ([]domain.TeamStats, []domain.TopCreator, []domain.Task, error) {
	const methodCtx = "service/reports.Data"
	a, err := s.repo.TeamStats(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	b, err := s.repo.TopCreators(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	c, err := s.repo.InvalidAssignees(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return a, b, c, nil
}
