package auth

import (
	"context"
	"errors"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
}

// Service performs auth-related domain operations.
type Service struct {
	repo              UserRepository
	signingKey        []byte
	issuer            string
	expirationMinutes int
}

func NewService(repo UserRepository, signingKey, issuer string, expirationMinutes int) *Service {
	return &Service{
		repo:              repo,
		signingKey:        []byte(signingKey),
		issuer:            issuer,
		expirationMinutes: expirationMinutes,
	}
}

func (s *Service) Register(ctx context.Context, email, password, displayName string) (*User, error) {
	if email == "" || password == "" {
		return nil, errors.New("email and password are required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  displayName,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (*User, error) {
	if email == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

func (s *Service) GenerateToken(user *User, role string) (string, error) {
	if user == nil {
		return "", errors.New("user is required")
	}

	now := time.Now().UTC()
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(s.expirationMinutes) * time.Minute)),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.signingKey)
}

func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, errors.New("token is required")
	}

	claims := &Claims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return s.signingKey, nil
	})
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	if claims.Issuer != s.issuer {
		return nil, errors.New("invalid token issuer")
	}

	if claims.UserID == "" || claims.Email == "" || claims.Role == "" {
		return nil, errors.New("token missing required claims")
	}

	return claims, nil
}
