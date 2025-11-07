package user

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Role definitions.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// User represents an account interacting with the gateway.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	IsBanned     bool      `json:"is_banned"`
	CreatedAt    time.Time `json:"created_at"`
}

var ErrNotFound = errors.New("user not found")

// Repository provides persistence helpers.
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, username, passwordHash, role string) (*User, error) {
	query := `INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`
	result, err := r.db.ExecContext(ctx, query, username, passwordHash, role)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return r.GetByID(ctx, id)
}

func (r *Repository) GetByUsername(ctx context.Context, username string) (*User, error) {
	return r.get(ctx, `SELECT id, username, password_hash, role, is_banned, created_at FROM users WHERE username = ?`, username)
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*User, error) {
	return r.get(ctx, `SELECT id, username, password_hash, role, is_banned, created_at FROM users WHERE id = ?`, id)
}

func (r *Repository) get(ctx context.Context, query string, args ...interface{}) (*User, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.IsBanned, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repository) List(ctx context.Context) ([]User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, role, is_banned, created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.IsBanned, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}

	return users, rows.Err()
}

func (r *Repository) SetBanState(ctx context.Context, userID int64, banned bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET is_banned = ? WHERE id = ?`, boolToInt(banned), userID)
	return err
}

func (r *Repository) CountAdmins(ctx context.Context) (int, error) {
	row := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ?`, RoleAdmin)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) UpdateRole(ctx context.Context, userID int64, role string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET role = ? WHERE id = ?`, role, userID)
	return err
}

func (r *Repository) UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
