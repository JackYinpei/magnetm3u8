package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// Session represents a persisted login token.
type Session struct {
	Token     string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Store persists sessions in SQLite.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, userID int64, ttl time.Duration) (*Session, error) {
	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(ttl)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, expiresAt)
	if err != nil {
		return nil, err
	}

	return &Session{
		Token:     token,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}, nil
}

func (s *Store) Get(ctx context.Context, token string) (*Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT token, user_id, expires_at, created_at FROM sessions WHERE token = ?`, token)
	var sess Session
	if err := row.Scan(&sess.Token, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if time.Now().After(sess.ExpiresAt) {
		_ = s.Delete(ctx, token)
		return nil, nil
	}

	return &sess, nil
}

func (s *Store) Delete(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func randomToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
