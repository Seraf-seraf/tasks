package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/email"
	"taskservice/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func newSvc(t *testing.T) (*Service, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return New(repository.New(db), nil, email.New("")), mock, func() { _ = db.Close() }
}

func TestRegisterHashesPassword(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectExec("INSERT INTO users").WithArgs("a@b.com", "Ann", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(7, 1))
	id, err := svc.Register(context.Background(), "a@b.com", "Ann", "secret")
	require.NoError(t, err)
	require.Equal(t, int64(7), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateTaskRequiresTeamMember(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).WillReturnError(sql.ErrNoRows)
	_, err := svc.CreateTask(context.Background(), domain.Task{TeamID: 10, CreatedBy: 2, Title: "x", Status: domain.StatusTodo})
	require.Error(t, err)
	require.Contains(t, err.Error(), "member required")
}

func TestUpdateTaskMemberForbiddenForUnrelatedTask(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	rows := sqlmock.NewRows([]string{"id", "team_id", "title", "description", "status", "assignee_id", "created_by", "created_at", "updated_at"}).
		AddRow(3, 10, "old", "d", "todo", nil, 5, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mock.ExpectQuery("SELECT id,team_id,title").WithArgs(int64(3)).WillReturnRows(rows)
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))
	err := svc.UpdateTask(context.Background(), 2, 3, domain.Task{Title: "new"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "forbidden")
}
