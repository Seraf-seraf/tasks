package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/Seraf-seraf/tasks/internal/domain"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func newRepo(t *testing.T) (*Repo, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return New(db), mock, func() { _ = db.Close() }
}

func TestTeamStatsUsesJoinAndAggregation(t *testing.T) {
	repo, mock, closeFn := newRepo(t)
	defer closeFn()
	mock.ExpectQuery("SELECT t.id,t.name,COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "members", "done"}).
			AddRow(10, "Platform", 2, 1))

	stats, err := repo.TeamStats(context.Background())

	require.NoError(t, err)
	require.Equal(t, []domain.TeamStats{{TeamID: 10, Name: "Platform", Members: 2, DoneLast7Days: 1}}, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTopCreatorsUsesWindowRanking(t *testing.T) {
	repo, mock, closeFn := newRepo(t)
	defer closeFn()
	mock.ExpectQuery("DENSE_RANK\\(\\) OVER").
		WillReturnRows(sqlmock.NewRows([]string{"team_id", "user_id", "email", "created_tasks", "rnk"}).
			AddRow(10, 7, "owner@example.com", 3, 1))

	top, err := repo.TopCreators(context.Background())

	require.NoError(t, err)
	require.Len(t, top, 1)
	require.Equal(t, int64(1), top[0].Rank)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInvalidAssigneesFindsRelatedTableMismatch(t *testing.T) {
	repo, mock, closeFn := newRepo(t)
	defer closeFn()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("LEFT JOIN team_members").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "title", "description", "status", "assignee_id", "created_by", "created_at", "updated_at"}).
			AddRow(33, 10, "bad", "d", "todo", int64(99), 7, now, now))

	tasks, err := repo.InvalidAssignees(context.Background())

	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, int64(99), *tasks[0].AssigneeID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTaskWritesAuditHistoryInTransaction(t *testing.T) {
	repo, mock, closeFn := newRepo(t)
	defer closeFn()
	old := domain.Task{ID: 33, Title: "old", Description: "d", Status: domain.StatusTodo}
	next := domain.Task{ID: 33, Title: "new", Description: "d", Status: domain.StatusDone}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET title=\\?,description=\\?,status=\\?,assignee_id=\\? WHERE id=\\?").
		WithArgs("new", "d", domain.StatusDone, nil, int64(33)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO task_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO task_history").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	err := repo.UpdateTask(context.Background(), old, next, 7)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
