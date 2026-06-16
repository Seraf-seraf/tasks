package service

import (
	"context"
	"fmt"

	"github.com/Seraf-seraf/tasks/internal/domain"
	"github.com/Seraf-seraf/tasks/internal/email"
	repository "github.com/Seraf-seraf/tasks/internal/repository/mysql"
)

type TeamService struct {
	repo  *repository.Repo
	email *email.Client
}

func NewTeamService(r *repository.Repo, e *email.Client) *TeamService {
	return &TeamService{repo: r, email: e}
}

func (s *TeamService) Create(ctx context.Context, name string, uid int64) (int64, error) {
	const methodCtx = "service/teams.Create"
	id, err := s.repo.CreateTeam(ctx, name, uid)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return id, nil
}

func (s *TeamService) ListForUser(ctx context.Context, uid int64) ([]domain.Team, error) {
	const methodCtx = "service/teams.ListForUser"
	teams, err := s.repo.TeamsForUser(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return teams, nil
}

func (s *TeamService) Invite(ctx context.Context, actor, teamID, userID int64, role domain.Role, emailAddr string) error {
	const methodCtx = "service/teams.Invite"
	r, err := s.repo.Role(ctx, teamID, actor)
	if err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	if r != domain.RoleOwner && r != domain.RoleAdmin {
		return fmt.Errorf("%s: forbidden", methodCtx)
	}
	if err := s.repo.Invite(ctx, teamID, userID, role); err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	return s.email.SendInvite(ctx, emailAddr)
}
