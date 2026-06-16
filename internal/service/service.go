package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"
	"taskservice/internal/domain"
	"taskservice/internal/email"
	"taskservice/internal/repository"
	"time"
)

type Service struct {
	repo  *repository.Repo
	redis *redis.Client
	email *email.Client
}

func New(r *repository.Repo, rc *redis.Client, e *email.Client) *Service { return &Service{r, rc, e} }
func (s *Service) Register(ctx context.Context, email, name, password string) (int64, error) {
	const methodCtx = "service.Register"
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return s.repo.CreateUser(ctx, domain.User{Email: email, Name: name, PasswordHash: string(h)})
}
func (s *Service) Login(ctx context.Context, email, password string) (domain.User, error) {
	const methodCtx = "service.Login"
	u, err := s.repo.UserByEmail(ctx, email)
	if err != nil {
		return u, fmt.Errorf("%s: %w", methodCtx, err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return u, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return u, nil
}
func (s *Service) CreateTeam(ctx context.Context, name string, uid int64) (int64, error) {
	return s.repo.CreateTeam(ctx, name, uid)
}
func (s *Service) Teams(ctx context.Context, uid int64) ([]domain.Team, error) {
	return s.repo.TeamsForUser(ctx, uid)
}
func (s *Service) Invite(ctx context.Context, actor, teamID, userID int64, role domain.Role, emailAddr string) error {
	const methodCtx = "service.Invite"
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
func (s *Service) CreateTask(ctx context.Context, t domain.Task) (int64, error) {
	const methodCtx = "service.CreateTask"
	if _, err := s.repo.Role(ctx, t.TeamID, t.CreatedBy); err != nil {
		return 0, fmt.Errorf("%s: member required: %w", methodCtx, err)
	}
	id, err := s.repo.CreateTask(ctx, t)
	if err == nil {
		s.invalidate(ctx, t.TeamID)
	}
	return id, err
}
func (s *Service) ListTasks(ctx context.Context, uid, teamID int64, status string, assignee *int64, limit, offset int) ([]domain.Task, error) {
	const methodCtx = "service.ListTasks"
	if _, err := s.repo.Role(ctx, teamID, uid); err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	key := repository.CacheKey(teamID, status, assignee, limit, offset)
	if s.redis != nil {
		if b, err := s.redis.Get(ctx, key).Bytes(); err == nil {
			var out []domain.Task
			if json.Unmarshal(b, &out) == nil {
				return out, nil
			}
		}
	}
	out, err := s.repo.ListTasks(ctx, teamID, status, assignee, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	if s.redis != nil {
		if b, err := json.Marshal(out); err == nil {
			_ = s.redis.Set(ctx, key, b, 5*time.Minute).Err()
		}
	}
	return out, nil
}
func (s *Service) UpdateTask(ctx context.Context, uid, id int64, patch domain.Task) error {
	const methodCtx = "service.UpdateTask"
	old, err := s.repo.TaskByID(ctx, id)
	if err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	role, err := s.repo.Role(ctx, old.TeamID, uid)
	if err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	if role == domain.RoleMember && old.CreatedBy != uid && (old.AssigneeID == nil || *old.AssigneeID != uid) {
		return fmt.Errorf("%s: forbidden", methodCtx)
	}
	patch.ID = id
	patch.TeamID = old.TeamID
	patch.CreatedBy = old.CreatedBy
	if patch.Title == "" {
		patch.Title = old.Title
	}
	if patch.Description == "" {
		patch.Description = old.Description
	}
	if patch.Status == "" {
		patch.Status = old.Status
	}
	if err := s.repo.UpdateTask(ctx, old, patch, uid); err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	s.invalidate(ctx, old.TeamID)
	return nil
}
func (s *Service) History(ctx context.Context, uid, taskID int64) ([]domain.TaskHistory, error) {
	const methodCtx = "service.History"
	t, err := s.repo.TaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	if _, err := s.repo.Role(ctx, t.TeamID, uid); err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return s.repo.History(ctx, taskID)
}
func (s *Service) Reports(ctx context.Context) ([]domain.TeamStats, []domain.TopCreator, []domain.Task, error) {
	const methodCtx = "service.Reports"
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
func (s *Service) invalidate(ctx context.Context, teamID int64) {
	if s.redis == nil {
		return
	}
	it := s.redis.Scan(ctx, 0, fmt.Sprintf("tasks:%d:*", teamID), 100).Iterator()
	for it.Next(ctx) {
		_ = s.redis.Del(ctx, it.Val()).Err()
	}
}

var ErrNoRows = sql.ErrNoRows
var ErrForbidden = errors.New("forbidden")
