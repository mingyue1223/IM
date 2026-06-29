package service

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/goim/goim/internal/middleware"
	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── Validation / business-error constants ──

const (
	ErrUsernameTooShort = "username must be 3-50 characters"
	ErrPasswordTooShort = "password must be at least 6 characters"
	ErrUsernameTaken    = "username is already taken"
	ErrUserNotFound     = "user not found"
	ErrWrongPassword    = "wrong password"
	ErrInvalidToken     = "invalid or expired refresh token"
)

// AuthService handles user registration, login, and token refresh.
type AuthService struct {
	repo            repository.MySQLRepo
	jwtSecret       string
	bcryptCost      int
	accessExpHours  int
	refreshExpDays  int
}

// NewAuthService creates an AuthService with the given MySQL repo and JWT config.
func NewAuthService(repo repository.MySQLRepo, jwtSecret string, accessExpHours, refreshExpDays int) *AuthService {
	return &AuthService{
		repo:            repo,
		jwtSecret:       jwtSecret,
		bcryptCost:      10,
		accessExpHours:  accessExpHours,
		refreshExpDays:  refreshExpDays,
	}
}

// Register validates input, hashes the password, and creates a new user.
// Returns the new user's ID and username on success.
func (s *AuthService) Register(ctx context.Context, username, password string) (int64, string, error) {
	// Validate username length
	if len(username) < 3 || len(username) > 50 {
		return 0, "", fmt.Errorf(ErrUsernameTooShort)
	}
	// Validate password length
	if len(password) < 6 {
		return 0, "", fmt.Errorf(ErrPasswordTooShort)
	}

	// Check username uniqueness
	existing, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return 0, "", fmt.Errorf("check username: %w", err)
	}
	if existing != nil {
		return 0, "", fmt.Errorf(ErrUsernameTaken)
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return 0, "", fmt.Errorf("hash password: %w", err)
	}

	// Create user row
	user := &model.User{
		Username:     username,
		PasswordHash: string(hash),
		Nickname:     username, // default nickname = username
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return 0, "", fmt.Errorf("create user: %w", err)
	}

	return user.ID, user.Username, nil
}

// Login validates credentials and returns JWT tokens.
// Returns accessToken, refreshToken, expiresIn (seconds), error.
func (s *AuthService) Login(ctx context.Context, username, password string) (string, string, int64, error) {
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return "", "", 0, fmt.Errorf("lookup user: %w", err)
	}
	if user == nil {
		return "", "", 0, fmt.Errorf(ErrUserNotFound)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", "", 0, fmt.Errorf(ErrWrongPassword)
	}

	// Generate access token
	accessToken, err := middleware.GenerateAccessToken(user.ID, user.Username, s.jwtSecret, s.accessExpHours)
	if err != nil {
		return "", "", 0, fmt.Errorf("generate access token: %w", err)
	}

	// Generate refresh token
	refreshToken, err := middleware.GenerateRefreshToken(user.ID, s.jwtSecret, s.refreshExpDays)
	if err != nil {
		return "", "", 0, fmt.Errorf("generate refresh token: %w", err)
	}

	expiresIn := int64(s.accessExpHours * 3600)
	return accessToken, refreshToken, expiresIn, nil
}

// Refresh validates a refresh token and issues a new access token.
// Returns accessToken, expiresIn (seconds), error.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (string, int64, error) {
	_, claims, err := middleware.ParseToken(refreshToken, s.jwtSecret)
	if err != nil {
		return "", 0, fmt.Errorf(ErrInvalidToken)
	}

	// Verify user still exists
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return "", 0, fmt.Errorf("lookup user: %w", err)
	}
	if user == nil {
		return "", 0, fmt.Errorf(ErrUserNotFound)
	}

	// Generate new access token
	accessToken, err := middleware.GenerateAccessToken(user.ID, user.Username, s.jwtSecret, s.accessExpHours)
	if err != nil {
		return "", 0, fmt.Errorf("generate access token: %w", err)
	}

	expiresIn := int64(s.accessExpHours * 3600)
	return accessToken, expiresIn, nil
}

// TokenExpiry returns the configured access-token expiry duration.
func (s *AuthService) TokenExpiry() time.Duration {
	return time.Duration(s.accessExpHours) * time.Hour
}
