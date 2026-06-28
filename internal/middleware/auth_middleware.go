package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims holds the JWT payload for GoIM tokens.
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenerateAccessToken creates a signed JWT access token with the given expiry.
// expireHours is the token lifetime in hours (typically 2).
func GenerateAccessToken(userID int64, username, secret string, expireHours int) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateRefreshToken creates a signed JWT refresh token with the given expiry.
// expireDays is the token lifetime in days (typically 7).
// Refresh tokens do not carry username — only userID is embedded.
func GenerateRefreshToken(userID int64, secret string, expireDays int) (string, error) {
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireDays) * 24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// JWTAuthMiddleware returns a Gin middleware that validates JWT tokens.
// It accepts tokens from:
//   - Authorization header (Bearer <token>)
//   - Query parameter "token" (for WebSocket upgrade requests)
//
// On success, it sets "userID" (int64) and "username" (string) in the Gin context.
// On failure, it aborts with 401 Unauthorized.
func JWTAuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// Also check query param for WebSocket connections
			token := c.Query("token")
			if token == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
				return
			}
			authHeader = "Bearer " + token
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims := &Claims{}
		_, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}
