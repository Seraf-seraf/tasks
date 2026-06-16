package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Seraf-seraf/tasks/internal/domain"
	"github.com/Seraf-seraf/tasks/internal/email"
	repository "github.com/Seraf-seraf/tasks/internal/repository/mysql"
	"github.com/go-redis/redis/v8"
)

type Service struct {
	AuthService   *AuthService
	TeamService   *TeamService
	TaskService   *TaskService
	ReportService *ReportService
}

func New(r *repository.Repo, rc *redis.Client, e *email.Client) *Service {
	cache := NewTaskCache(rc)
	return &Service{
		AuthService:   NewAuthService(r),
		TeamService:   NewTeamService(r, e),
		TaskService:   NewTaskService(r, cache),
		ReportService: NewReportService(r),
	}
}

func (s *Service) Register(ctx context.Context, email, name, password string) (int64, error) {
	const methodCtx = "service.Register"
	id, err := s.AuthService.Register(ctx, email, name, password)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return id, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (domain.User, error) {
	const methodCtx = "service.Login"
	u, err := s.AuthService.Login(ctx, email, password)
	if err != nil {
		return u, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return u, nil
}

func (s *Service) CreateTeam(ctx context.Context, name string, uid int64) (int64, error) {
	const methodCtx = "service.CreateTeam"
	id, err := s.TeamService.Create(ctx, name, uid)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return id, nil
}

func (s *Service) TeamsForUser(ctx context.Context, uid int64) ([]domain.Team, error) {
	const methodCtx = "service.TeamsForUser"
	teams, err := s.TeamService.ListForUser(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return teams, nil
}

func (s *Service) Teams(ctx context.Context, uid int64) ([]domain.Team, error) {
	return s.TeamsForUser(ctx, uid)
}

func (s *Service) Invite(ctx context.Context, actor, teamID, userID int64, role domain.Role, emailAddr string) error {
	const methodCtx = "service.Invite"
	if err := s.TeamService.Invite(ctx, actor, teamID, userID, role, emailAddr); err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	return nil
}

func (s *Service) CreateTask(ctx context.Context, t domain.Task) (int64, error) {
	const methodCtx = "service.CreateTask"
	id, err := s.TaskService.Create(ctx, t)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return id, nil
}

func (s *Service) ListTasks(ctx context.Context, uid, teamID int64, status string, assignee *int64, limit, offset int) ([]domain.Task, error) {
	const methodCtx = "service.ListTasks"
	tasks, err := s.TaskService.List(ctx, uid, teamID, status, assignee, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return tasks, nil
}

func (s *Service) UpdateTask(ctx context.Context, uid, id int64, patch domain.Task) error {
	const methodCtx = "service.UpdateTask"
	if err := s.TaskService.Update(ctx, uid, id, patch); err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	return nil
}

func (s *Service) History(ctx context.Context, uid, taskID int64) ([]domain.TaskHistory, error) {
	const methodCtx = "service.History"
	history, err := s.TaskService.History(ctx, uid, taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return history, nil
}

func (s *Service) ReportsData(ctx context.Context) ([]domain.TeamStats, []domain.TopCreator, []domain.Task, error) {
	const methodCtx = "service.ReportsData"
	stats, top, invalid, err := s.ReportService.Data(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return stats, top, invalid, nil
}

func (s *Service) Reports(ctx context.Context) ([]domain.TeamStats, []domain.TopCreator, []domain.Task, error) {
	return s.ReportsData(ctx)
}

var ErrNoRows = sql.ErrNoRows
var ErrForbidden = errors.New("forbidden")
