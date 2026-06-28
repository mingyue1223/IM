package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestJWTAuthMiddleware_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	token, err := GenerateAccessToken(1, "testuser", "test-secret", 2)
	assert.NoError(t, err)

	r := gin.New()
	r.Use(JWTAuthMiddleware("test-secret"))
	r.GET("/test", func(c *gin.Context) {
		userID := c.GetInt64("userID")
		username := c.GetString("username")
		c.JSON(200, gin.H{"userID": userID, "username": username})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

func TestJWTAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(JWTAuthMiddleware("test-secret"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func TestJWTAuthMiddleware_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(JWTAuthMiddleware("test-secret"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func TestJWTAuthMiddleware_QueryParamToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	token, err := GenerateAccessToken(42, "wsuser", "ws-secret", 2)
	assert.NoError(t, err)

	r := gin.New()
	r.Use(JWTAuthMiddleware("ws-secret"))
	r.GET("/ws", func(c *gin.Context) {
		userID := c.GetInt64("userID")
		username := c.GetString("username")
		c.JSON(200, gin.H{"userID": userID, "username": username})
	})

	req := httptest.NewRequest("GET", "/ws?token="+token, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

func TestGenerateAccessToken(t *testing.T) {
	token, err := GenerateAccessToken(100, "alice", "mysecret", 2)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateRefreshToken(t *testing.T) {
	token, err := GenerateRefreshToken(100, "mysecret", 7)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}
