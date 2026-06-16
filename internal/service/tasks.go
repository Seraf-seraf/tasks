package service

import (
	"context"
	"fmt"

	"github.com/Seraf-seraf/tasks/internal/domain"
	repository "github.com/Seraf-seraf/tasks/internal/repository/mysql"
)

type TaskService struct {
	repo  *repository.Repo
	cache *TaskCache
}

func NewTaskService(r *repository.Repo, cache *TaskCache) *TaskService {
	return &TaskService{repo: r, cache: cache}
}

func (s *TaskService) Create(ctx context.Context, t domain.Task) (int64, error) {
	const methodCtx = "service/tasks.Create"
	if _, err := s.repo.Role(ctx, t.TeamID, t.CreatedBy); err != nil {
		return 0, fmt.Errorf("%s: member required: %w", methodCtx, err)
	}
	id, err := s.repo.CreateTask(ctx, t)
	if err == nil {
		s.cache.Invalidate(ctx, t.TeamID)
	}
	return id, err
}

func (s *TaskService) List(ctx context.Context, uid, teamID int64, status string, assignee *int64, limit, offset int) ([]domain.Task, error) {
	const methodCtx = "service/tasks.List"
	if _, err := s.repo.Role(ctx, teamID, uid); err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	if out, ok := s.cache.Get(ctx, teamID, status, assignee, limit, offset); ok {
		return out, nil
	}
	out, err := s.repo.ListTasks(ctx, teamID, status, assignee, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	s.cache.Set(ctx, teamID, status, assignee, limit, offset, out)
	return out, nil
}

func (s *TaskService) Update(ctx context.Context, uid, id int64, patch domain.Task) error {
	const methodCtx = "service/tasks.Update"
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
	s.cache.Invalidate(ctx, old.TeamID)
	return nil
}

func (s *TaskService) History(ctx context.Context, uid, taskID int64) ([]domain.TaskHistory, error) {
	const methodCtx = "service/tasks.History"
	t, err := s.repo.TaskByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	if _, err := s.repo.Role(ctx, t.TeamID, uid); err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return s.repo.History(ctx, taskID)
}
