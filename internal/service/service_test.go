package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Seraf-seraf/tasks/internal/domain"
	"github.com/Seraf-seraf/tasks/internal/email"
	repository "github.com/Seraf-seraf/tasks/internal/repository/mysql"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
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

func TestLoginComparesPasswordHash(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	require.NoError(t, err)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT id,email,name,password_hash,created_at FROM users WHERE email=\\?").
		WithArgs("a@b.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "password_hash", "created_at"}).
			AddRow(7, "a@b.com", "Ann", string(hash), now))

	u, err := svc.Login(context.Background(), "a@b.com", "secret")

	require.NoError(t, err)
	require.Equal(t, int64(7), u.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT id,email,name,password_hash,created_at FROM users WHERE email=\\?").
		WithArgs("a@b.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "password_hash", "created_at"}).
			AddRow(7, "a@b.com", "Ann", string(hash), time.Now()))

	_, err = svc.Login(context.Background(), "a@b.com", "wrong")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateTeamAddsOwnerMembership(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO teams").WithArgs("Platform", int64(7)).WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec("INSERT INTO team_members").WithArgs(int64(7), int64(10), domain.RoleOwner).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	id, err := svc.CreateTeam(context.Background(), "Platform", 7)

	require.NoError(t, err)
	require.Equal(t, int64(10), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamsForUserReturnsMembershipRole(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT t.id,t.name,t.created_by,tm.role,t.created_at FROM teams").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_by", "role", "created_at"}).
			AddRow(10, "Platform", 7, "owner", now))

	teams, err := svc.TeamsForUser(context.Background(), 7)

	require.NoError(t, err)
	require.Len(t, teams, 1)
	require.Equal(t, domain.RoleOwner, teams[0].Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamsAliasCallsTeamsForUser(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT t.id,t.name,t.created_by,tm.role,t.created_at FROM teams").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_by", "role", "created_at"}).
			AddRow(10, "Platform", 7, "owner", now))

	teams, err := svc.Teams(context.Background(), 7)

	require.NoError(t, err)
	require.Len(t, teams, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInviteAllowsAdmin(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("admin"))
	mock.ExpectExec("INSERT INTO team_members").WithArgs(int64(5), int64(10), domain.RoleMember).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := svc.Invite(context.Background(), 2, 10, 5, domain.RoleMember, "")

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInvitePropagatesRepositoryError(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("owner"))
	mock.ExpectExec("INSERT INTO team_members").WithArgs(int64(5), int64(10), domain.RoleMember).
		WillReturnError(sql.ErrConnDone)

	err := svc.Invite(context.Background(), 2, 10, 5, domain.RoleMember, "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "sql: connection is already closed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInviteRejectsMember(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))

	err := svc.Invite(context.Background(), 2, 10, 5, domain.RoleMember, "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "forbidden")
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

func TestCreateTaskPersistsForTeamMember(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	assigneeID := int64(4)
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))
	mock.ExpectExec("INSERT INTO tasks").
		WithArgs(int64(10), "x", "d", domain.StatusTodo, &assigneeID, int64(2)).
		WillReturnResult(sqlmock.NewResult(33, 1))

	id, err := svc.CreateTask(context.Background(), domain.Task{
		TeamID: 10, CreatedBy: 2, Title: "x", Description: "d", Status: domain.StatusTodo, AssigneeID: &assigneeID,
	})

	require.NoError(t, err)
	require.Equal(t, int64(33), id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListTasksChecksMembershipAndPaginates(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	assigneeID := int64(4)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))
	mock.ExpectQuery("SELECT id,team_id,title,description,status,assignee_id,created_by,created_at,updated_at FROM tasks WHERE team_id=\\? AND status=\\? AND assignee_id=\\? ORDER BY id DESC LIMIT \\? OFFSET \\?").
		WithArgs(int64(10), "todo", assigneeID, 20, 40).
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "title", "description", "status", "assignee_id", "created_by", "created_at", "updated_at"}).
			AddRow(33, 10, "x", "d", "todo", assigneeID, 2, now, now))

	tasks, err := svc.ListTasks(context.Background(), 2, 10, "todo", &assigneeID, 20, 40)

	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, assigneeID, *tasks[0].AssigneeID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListTasksRejectsNonMember(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnError(sql.ErrNoRows)

	tasks, err := svc.ListTasks(context.Background(), 2, 10, "", nil, 20, 0)

	require.Error(t, err)
	require.Nil(t, tasks)
	require.NoError(t, mock.ExpectationsWereMet())
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

func TestUpdateTaskReturnsNotFound(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT id,team_id,title").WithArgs(int64(3)).WillReturnError(sql.ErrNoRows)

	err := svc.UpdateTask(context.Background(), 2, 3, domain.Task{Title: "new"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTaskAllowsAssigneeAndWritesHistory(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	assigneeID := int64(2)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT id,team_id,title").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "title", "description", "status", "assignee_id", "created_by", "created_at", "updated_at"}).
			AddRow(3, 10, "old", "d", "todo", assigneeID, 5, now, now))
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET title=\\?,description=\\?,status=\\?,assignee_id=\\? WHERE id=\\?").
		WithArgs("new", "d", domain.StatusDone, &assigneeID, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO task_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO task_history").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	err := svc.UpdateTask(context.Background(), 2, 3, domain.Task{
		Title: "new", Status: domain.StatusDone, AssigneeID: &assigneeID,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTaskPreservesEmptyPatchFields(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT id,team_id,title").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "title", "description", "status", "assignee_id", "created_by", "created_at", "updated_at"}).
			AddRow(3, 10, "old", "d", "todo", nil, 2, now, now))
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("owner"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET title=\\?,description=\\?,status=\\?,assignee_id=\\? WHERE id=\\?").
		WithArgs("old", "d", domain.StatusTodo, nil, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := svc.UpdateTask(context.Background(), 2, 3, domain.Task{})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHistoryChecksTeamMembership(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT id,team_id,title").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "title", "description", "status", "assignee_id", "created_by", "created_at", "updated_at"}).
			AddRow(3, 10, "old", "d", "todo", nil, 5, now, now))
	mock.ExpectQuery("SELECT role FROM team_members").WithArgs(int64(10), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("member"))
	mock.ExpectQuery("SELECT id,task_id,changed_by,field,old_value,new_value,created_at FROM task_history").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "changed_by", "field", "old_value", "new_value", "created_at"}).
			AddRow(1, 3, 2, "title", "old", "new", now))

	history, err := svc.History(context.Background(), 2, 3)

	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "title", history[0].Field)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHistoryReturnsTaskLookupError(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT id,team_id,title").WithArgs(int64(3)).WillReturnError(sql.ErrNoRows)

	history, err := svc.History(context.Background(), 2, 3)

	require.Error(t, err)
	require.Nil(t, history)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReportsAggregatesRepositoryQueries(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT t.id,t.name,COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "members", "done"}).
			AddRow(10, "Platform", 2, 1))
	mock.ExpectQuery("SELECT team_id,user_id,email,created_tasks,rnk FROM").
		WillReturnRows(sqlmock.NewRows([]string{"team_id", "user_id", "email", "created_tasks", "rnk"}).
			AddRow(10, 7, "owner@example.com", 3, 1))
	mock.ExpectQuery("SELECT ta.id,ta.team_id,ta.title,ta.description,ta.status,ta.assignee_id,ta.created_by,ta.created_at,ta.updated_at FROM tasks ta").
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "title", "description", "status", "assignee_id", "created_by", "created_at", "updated_at"}).
			AddRow(33, 10, "bad", "d", "todo", int64(99), 7, now, now))

	stats, top, invalid, err := svc.Reports(context.Background())

	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Len(t, top, 1)
	require.Len(t, invalid, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReportsReturnsTeamStatsError(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT t.id,t.name,COUNT").WillReturnError(sql.ErrConnDone)

	_, _, _, err := svc.Reports(context.Background())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReportsReturnsTopCreatorsError(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT t.id,t.name,COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "members", "done"}))
	mock.ExpectQuery("SELECT team_id,user_id,email,created_tasks,rnk FROM").
		WillReturnError(sql.ErrConnDone)

	_, _, _, err := svc.Reports(context.Background())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReportsReturnsInvalidAssigneesError(t *testing.T) {
	svc, mock, closeFn := newSvc(t)
	defer closeFn()
	mock.ExpectQuery("SELECT t.id,t.name,COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "members", "done"}))
	mock.ExpectQuery("SELECT team_id,user_id,email,created_tasks,rnk FROM").
		WillReturnRows(sqlmock.NewRows([]string{"team_id", "user_id", "email", "created_tasks", "rnk"}))
	mock.ExpectQuery("SELECT ta.id,ta.team_id,ta.title,ta.description,ta.status,ta.assignee_id,ta.created_by,ta.created_at,ta.updated_at FROM tasks ta").
		WillReturnError(sql.ErrConnDone)

	_, _, _, err := svc.Reports(context.Background())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskCacheNilRedisIsNoop(t *testing.T) {
	cache := NewTaskCache(nil)

	tasks, ok := cache.Get(context.Background(), 10, "todo", nil, 20, 0)
	cache.Set(context.Background(), 10, "todo", nil, 20, 0, []domain.Task{{ID: 1}})
	cache.Invalidate(context.Background(), 10)

	require.False(t, ok)
	require.Nil(t, tasks)
}

func TestTaskCacheRedisErrorsAreIgnored(t *testing.T) {
	cache := NewTaskCache(redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}))
	ctx := context.Background()

	tasks, ok := cache.Get(ctx, 10, "todo", nil, 20, 0)
	cache.Set(ctx, 10, "todo", nil, 20, 0, []domain.Task{{ID: 1}})
	cache.Invalidate(ctx, 10)

	require.False(t, ok)
	require.Nil(t, tasks)
}
