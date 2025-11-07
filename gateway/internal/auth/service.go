package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"magnetm3u8-gateway/internal/session"
	"magnetm3u8-gateway/internal/user"
)

// Service encapsulates registration, authentication, and session workflows.
type Service struct {
	users    *user.Repository
	sessions *session.Store
	ttl      time.Duration
}

func NewService(userRepo *user.Repository, sessionStore *session.Store, ttl time.Duration) *Service {
	return &Service{
		users:    userRepo,
		sessions: sessionStore,
		ttl:      ttl,
	}
}

func (s *Service) Register(ctx context.Context, username, password string) (*user.User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 {
		return nil, fmt.Errorf("用户名至少3个字符")
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("密码至少6个字符")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return s.users.Create(ctx, username, string(hash), user.RoleUser)
}

func (s *Service) Login(ctx context.Context, username, password string) (string, *user.User, error) {
	account, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return "", nil, err
	}

	if account.IsBanned {
		return "", nil, errors.New("账号已被封禁")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)); err != nil {
		return "", nil, errors.New("用户名或密码错误")
	}

	session, err := s.sessions.Create(ctx, account.ID, s.ttl)
	if err != nil {
		return "", nil, err
	}

	return session.Token, account, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, token)
}

func (s *Service) UserFromToken(ctx context.Context, token string) (*user.User, error) {
	if token == "" {
		return nil, nil
	}

	session, err := s.sessions.Get(ctx, token)
	if err != nil || session == nil {
		return nil, err
	}

	account, err := s.users.GetByID(ctx, session.UserID)
	if errors.Is(err, user.ErrNotFound) {
		return nil, nil
	}
	return account, err
}

// EnsureDefaultAdmin creates or updates the default admin account.
func (s *Service) EnsureDefaultAdmin(ctx context.Context, username, password string) error {
	account, err := s.users.GetByUsername(ctx, username)
	if err != nil && !errors.Is(err, user.ErrNotFound) {
		return err
	}

	if errors.Is(err, user.ErrNotFound) {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return hashErr
		}
		_, createErr := s.users.Create(ctx, username, string(hash), user.RoleAdmin)
		return createErr
	}

	if account.Role != user.RoleAdmin {
		if updateErr := s.users.UpdateRole(ctx, account.ID, user.RoleAdmin); updateErr != nil {
			return updateErr
		}
	}

	return nil
}
