package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"taskservice/internal/domain"
)

type Repo struct{ db *sql.DB }

func New(db *sql.DB) *Repo { return &Repo{db: db} }
func (r *Repo) CreateUser(ctx context.Context, u domain.User) (int64, error) {
	const methodCtx = "repository.CreateUser"
	res, err := r.db.ExecContext(ctx, "INSERT INTO users(email,name,password_hash) VALUES(?,?,?)", u.Email, u.Name, u.PasswordHash)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return id, nil
}
func (r *Repo) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	const methodCtx = "repository.UserByEmail"
	var u domain.User
	err := r.db.QueryRowContext(ctx, "SELECT id,email,name,password_hash,created_at FROM users WHERE email=?", email).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return u, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return u, nil
}
func (r *Repo) CreateTeam(ctx context.Context, name string, uid int64) (int64, error) {
	const methodCtx = "repository.CreateTeam"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "INSERT INTO teams(name,created_by) VALUES(?,?)", name, uid)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	id, _ := res.LastInsertId()
	_, err = tx.ExecContext(ctx, "INSERT INTO team_members(user_id,team_id,role) VALUES(?,?,?)", uid, id, domain.RoleOwner)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return id, nil
}
func (r *Repo) TeamsForUser(ctx context.Context, uid int64) ([]domain.Team, error) {
	const methodCtx = "repository.TeamsForUser"
	rows, err := r.db.QueryContext(ctx, "SELECT t.id,t.name,t.created_by,tm.role,t.created_at FROM teams t JOIN team_members tm ON tm.team_id=t.id WHERE tm.user_id=? ORDER BY t.id DESC", uid)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	defer rows.Close()
	var out []domain.Team
	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedBy, &t.Role, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("%s: %w", methodCtx, err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return out, nil
}
func (r *Repo) Role(ctx context.Context, teamID, uid int64) (domain.Role, error) {
	const methodCtx = "repository.Role"
	var role domain.Role
	err := r.db.QueryRowContext(ctx, "SELECT role FROM team_members WHERE team_id=? AND user_id=?", teamID, uid).Scan(&role)
	if err != nil {
		return "", fmt.Errorf("%s: %w", methodCtx, err)
	}
	return role, nil
}
func (r *Repo) Invite(ctx context.Context, teamID, userID int64, role domain.Role) error {
	const methodCtx = "repository.Invite"
	_, err := r.db.ExecContext(ctx, "INSERT INTO team_members(user_id,team_id,role) VALUES(?,?,?) ON DUPLICATE KEY UPDATE role=VALUES(role)", userID, teamID, role)
	if err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	return nil
}
func (r *Repo) CreateTask(ctx context.Context, t domain.Task) (int64, error) {
	const methodCtx = "repository.CreateTask"
	res, err := r.db.ExecContext(ctx, "INSERT INTO tasks(team_id,title,description,status,assignee_id,created_by) VALUES(?,?,?,?,?,?)", t.TeamID, t.Title, t.Description, t.Status, t.AssigneeID, t.CreatedBy)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return id, nil
}
func (r *Repo) ListTasks(ctx context.Context, teamID int64, status string, assignee *int64, limit, offset int) ([]domain.Task, error) {
	const methodCtx = "repository.ListTasks"
	q := "SELECT id,team_id,title,description,status,assignee_id,created_by,created_at,updated_at FROM tasks WHERE team_id=?"
	args := []any{teamID}
	if status != "" {
		q += " AND status=?"
		args = append(args, status)
	}
	if assignee != nil {
		q += " AND assignee_id=?"
		args = append(args, *assignee)
	}
	q += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	defer rows.Close()
	return scanTasks(methodCtx, rows)
}
func scanTasks(methodCtx string, rows *sql.Rows) ([]domain.Task, error) {
	var out []domain.Task
	for rows.Next() {
		var t domain.Task
		var a sql.NullInt64
		if err := rows.Scan(&t.ID, &t.TeamID, &t.Title, &t.Description, &t.Status, &a, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s: %w", methodCtx, err)
		}
		if a.Valid {
			t.AssigneeID = &a.Int64
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	return out, nil
}

var ErrNotFound = errors.New("not found")

func (r *Repo) TaskByID(ctx context.Context, id int64) (domain.Task, error) {
	const methodCtx = "repository.TaskByID"
	var t domain.Task
	var a sql.NullInt64
	err := r.db.QueryRowContext(ctx, "SELECT id,team_id,title,description,status,assignee_id,created_by,created_at,updated_at FROM tasks WHERE id=?", id).Scan(&t.ID, &t.TeamID, &t.Title, &t.Description, &t.Status, &a, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, fmt.Errorf("%s: %w", methodCtx, err)
	}
	if a.Valid {
		t.AssigneeID = &a.Int64
	}
	return t, nil
}
func (r *Repo) UpdateTask(ctx context.Context, old, t domain.Task, by int64) error {
	const methodCtx = "repository.UpdateTask"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "UPDATE tasks SET title=?,description=?,status=?,assignee_id=? WHERE id=?", t.Title, t.Description, t.Status, t.AssigneeID, t.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	changes := map[string][2]string{}
	if old.Title != t.Title {
		changes["title"] = [2]string{old.Title, t.Title}
	}
	if old.Description != t.Description {
		changes["description"] = [2]string{old.Description, t.Description}
	}
	if old.Status != t.Status {
		changes["status"] = [2]string{string(old.Status), string(t.Status)}
	}
	for f, v := range changes {
		if _, err := tx.ExecContext(ctx, "INSERT INTO task_history(task_id,changed_by,field,old_value,new_value) VALUES(?,?,?,?,?)", t.ID, by, f, v[0], v[1]); err != nil {
			return fmt.Errorf("%s: %w", methodCtx, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: %w", methodCtx, err)
	}
	return nil
}
func (r *Repo) History(ctx context.Context, taskID int64) ([]domain.TaskHistory, error) {
	const methodCtx = "repository.History"
	rows, err := r.db.QueryContext(ctx, "SELECT id,task_id,changed_by,field,old_value,new_value,created_at FROM task_history WHERE task_id=? ORDER BY id DESC", taskID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	defer rows.Close()
	var out []domain.TaskHistory
	for rows.Next() {
		var h domain.TaskHistory
		if err := rows.Scan(&h.ID, &h.TaskID, &h.ChangedBy, &h.Field, &h.OldValue, &h.NewValue, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("%s: %w", methodCtx, err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
func (r *Repo) TeamStats(ctx context.Context) ([]domain.TeamStats, error) {
	const methodCtx = "repository.TeamStats"
	rows, err := r.db.QueryContext(ctx, `SELECT t.id,t.name,COUNT(DISTINCT tm.user_id),COUNT(DISTINCT CASE WHEN ta.status='done' AND ta.updated_at >= NOW() - INTERVAL 7 DAY THEN ta.id END) FROM teams t LEFT JOIN team_members tm ON tm.team_id=t.id LEFT JOIN tasks ta ON ta.team_id=t.id GROUP BY t.id,t.name`)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	defer rows.Close()
	var out []domain.TeamStats
	for rows.Next() {
		var s domain.TeamStats
		if err := rows.Scan(&s.TeamID, &s.Name, &s.Members, &s.DoneLast7Days); err != nil {
			return nil, fmt.Errorf("%s: %w", methodCtx, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (r *Repo) TopCreators(ctx context.Context) ([]domain.TopCreator, error) {
	const methodCtx = "repository.TopCreators"
	rows, err := r.db.QueryContext(ctx, `SELECT team_id,user_id,email,created_tasks,rnk FROM (SELECT t.team_id,u.id user_id,u.email,COUNT(*) created_tasks,DENSE_RANK() OVER(PARTITION BY t.team_id ORDER BY COUNT(*) DESC) rnk FROM tasks t JOIN users u ON u.id=t.created_by WHERE t.created_at >= NOW() - INTERVAL 1 MONTH GROUP BY t.team_id,u.id,u.email) x WHERE rnk<=3 ORDER BY team_id,rnk`)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	defer rows.Close()
	var out []domain.TopCreator
	for rows.Next() {
		var x domain.TopCreator
		if err := rows.Scan(&x.TeamID, &x.UserID, &x.Email, &x.CreatedTasks, &x.Rank); err != nil {
			return nil, fmt.Errorf("%s: %w", methodCtx, err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repo) InvalidAssignees(ctx context.Context) ([]domain.Task, error) {
	const methodCtx = "repository.InvalidAssignees"
	rows, err := r.db.QueryContext(ctx, `SELECT ta.id,ta.team_id,ta.title,ta.description,ta.status,ta.assignee_id,ta.created_by,ta.created_at,ta.updated_at FROM tasks ta LEFT JOIN team_members tm ON tm.team_id=ta.team_id AND tm.user_id=ta.assignee_id WHERE ta.assignee_id IS NOT NULL AND tm.user_id IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	}
	defer rows.Close()
	return scanTasks(methodCtx, rows)
}
func CacheKey(teamID int64, status string, assignee *int64, limit, offset int) string {
	return fmt.Sprintf("tasks:%d:%s:%v:%d:%d", teamID, status, assignee, limit, offset)
}
func CleanSQL(s string) string { return strings.Join(strings.Fields(s), " ") }
