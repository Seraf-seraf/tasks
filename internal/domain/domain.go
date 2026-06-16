package domain

import "time"

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}
type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedBy int64     `json:"created_by"`
	Role      Role      `json:"role,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type TaskStatus string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
)

type Task struct {
	ID          int64      `json:"id"`
	TeamID      int64      `json:"team_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	AssigneeID  *int64     `json:"assignee_id"`
	CreatedBy   int64      `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type TaskHistory struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	ChangedBy int64     `json:"changed_by"`
	Field     string    `json:"field"`
	OldValue  string    `json:"old_value"`
	NewValue  string    `json:"new_value"`
	CreatedAt time.Time `json:"created_at"`
}
type TeamStats struct {
	TeamID        int64  `json:"team_id"`
	Name          string `json:"name"`
	Members       int64  `json:"members"`
	DoneLast7Days int64  `json:"done_last_7_days"`
}
type TopCreator struct {
	TeamID       int64  `json:"team_id"`
	UserID       int64  `json:"user_id"`
	Email        string `json:"email"`
	CreatedTasks int64  `json:"created_tasks"`
	Rank         int64  `json:"rank"`
}
