//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/Seraf-seraf/tasks/internal/domain"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	mysqltc "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func TestRepoReportsWithMySQLContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available for testcontainers integration test")
	}
	defer func() {
		if r := recover(); r != nil {
			msg := strings.ToLower(strings.TrimSpace(fmt.Sprint(r)))
			if strings.Contains(msg, "docker") || strings.Contains(msg, "xdg_runtime_dir") || strings.Contains(msg, "permission denied") {
				t.Skipf("docker daemon is not available for testcontainers integration test: %v", r)
			}
			panic(r)
		}
	}()

	ctx := context.Background()
	container, err := mysqltc.Run(ctx, "mysql:8.4", mysqltc.WithDatabase("tasks"), mysqltc.WithUsername("task"), mysqltc.WithPassword("task"))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "docker") {
			t.Skipf("docker daemon is not available for testcontainers integration test: %v", err)
		}
		require.NoError(t, err)
	}
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })

	dsn, err := container.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	require.NoError(t, err)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.NoError(t, goose.SetDialect("mysql"))
	require.NoError(t, goose.Up(db, "../../migrations"))

	repo := New(db)
	ownerID, err := repo.CreateUser(ctx, domain.User{Email: "owner@example.com", Name: "Owner", PasswordHash: "hash"})
	require.NoError(t, err)
	memberID, err := repo.CreateUser(ctx, domain.User{Email: "member@example.com", Name: "Member", PasswordHash: "hash"})
	require.NoError(t, err)
	outsiderID, err := repo.CreateUser(ctx, domain.User{Email: "outsider@example.com", Name: "Outsider", PasswordHash: "hash"})
	require.NoError(t, err)
	teamID, err := repo.CreateTeam(ctx, "Platform", ownerID)
	require.NoError(t, err)
	require.NoError(t, repo.Invite(ctx, teamID, memberID, domain.RoleMember))
	_, err = repo.CreateTask(ctx, domain.Task{TeamID: teamID, Title: "Done", Description: "d", Status: domain.StatusDone, AssigneeID: &memberID, CreatedBy: ownerID})
	require.NoError(t, err)
	_, err = repo.CreateTask(ctx, domain.Task{TeamID: teamID, Title: "Invalid", Description: "d", Status: domain.StatusTodo, AssigneeID: &outsiderID, CreatedBy: ownerID})
	require.NoError(t, err)

	stats, err := repo.TeamStats(ctx)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, int64(2), stats[0].Members)
	require.Equal(t, int64(1), stats[0].DoneLast7Days)

	top, err := repo.TopCreators(ctx)
	require.NoError(t, err)
	require.Len(t, top, 1)
	require.Equal(t, ownerID, top[0].UserID)

	invalid, err := repo.InvalidAssignees(ctx)
	require.NoError(t, err)
	require.Len(t, invalid, 1)
	require.Equal(t, outsiderID, *invalid[0].AssigneeID)
}
