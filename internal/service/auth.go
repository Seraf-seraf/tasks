package service

import (
	"context"
	"fmt"

	"github.com/Seraf-seraf/tasks/internal/domain"
	repository "github.com/Seraf-seraf/tasks/internal/repository/mysql"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	repo *repository.Repo
}

func NewAuthService(r *repository.Repo) *AuthService {
	return &AuthService{repo: r}
}

func (s *AuthService) Register(ctx context.Context, email, name, password string) (int64, error) {
	const methodCtx = "service/auth.Register"
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return s.repo.CreateUser(ctx, domain.User{Email: email, Name: name, PasswordHash: string(h)})
}

func (s *AuthService) Login(ctx context.Context, email, password string) (domain.User, error) {
	const methodCtx = "service/auth.Login"
	u, err := s.repo.UserByEmail(ctx, email)
	if err != nil {
		return u, fmt.Errorf("%s: %w", methodCtx, err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return u, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return u, nil
}
