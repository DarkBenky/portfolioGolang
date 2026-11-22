package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"portfolio/internal/store"
)

const (
	maxLoginAttempts = 5
	loginBlockWindow = 15 * time.Minute
)

func (s *Server) hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password + s.Cfg.Salt))
	return hex.EncodeToString(sum[:])
}

func (s *Server) token(userID string, email string) (string, error) {
	claims := &Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.Cfg.JWTExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.Cfg.JWTSecret))
}

func validEmail(email string) bool {
	if len(email) < 5 || len(email) > 254 {
		return false
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	dot := strings.LastIndexByte(email, '.')
	return dot > at+1 && dot < len(email)-1
}

func (s *Server) register(c echo.Context) error {
	var request struct {
		UserName string `json:"user_name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if !validEmail(request.Email) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid email format"})
	}
	if len(request.Password) < 8 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Password must be at least 8 characters"})
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(s.hashPassword(request.Password)), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error hashing password"})
	}
	err = s.DB.AddUser(store.User{
		ID: store.NewID(), UserName: request.UserName, Email: request.Email, Password: string(hashed),
	})
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Email already registered"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "User added successfully"})
}

func (s *Server) login(c echo.Context) error {
	if !s.allowLogin(c.RealIP()) {
		return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "Too many login attempts. Try again later."})
	}
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind(&request); err != nil || request.Email == "" || request.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Email and password are required"})
	}
	user, err := s.DB.GetUserByEmail(request.Email)
	if err != nil || user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(s.hashPassword(request.Password))) != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
	}
	token, err := s.token(user.ID, user.Email)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error generating token"})
	}
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	c.Response().Header().Set("X-Frame-Options", "DENY")
	return c.JSON(http.StatusOK, map[string]string{"token": token, "email": user.Email, "message": "Login successful"})
}

func (s *Server) allowLogin(ip string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	cutoff := time.Now().Add(-loginBlockWindow)
	var recent []time.Time
	for _, attempt := range s.loginAttempts[ip] {
		if attempt.After(cutoff) {
			recent = append(recent, attempt)
		}
	}
	if len(recent) >= maxLoginAttempts {
		s.loginAttempts[ip] = recent
		return false
	}
	s.loginAttempts[ip] = append(recent, time.Now())
	return true
}

func (s *Server) profile(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"user_id": s.userID(c), "email": s.email(c)})
}

func (s *Server) health(c echo.Context) error {
	if err := s.DB.Ping(); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
}

func (s *Server) newsExists(c echo.Context) error {
	exists, err := s.DB.NewsExists(c.QueryParam("title"), c.QueryParam("summary"), c.QueryParam("text"))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error checking news existence")
	}
	return c.JSON(http.StatusOK, map[string]bool{"exists": exists})
}
