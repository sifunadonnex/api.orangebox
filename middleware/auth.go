package middleware

import (
	"fdm-backend/config"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// AuthenticateToken validates the bearer token
func AuthenticateToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		bearer := config.GetBearerToken()

		if authHeader == "" {
			// Serve HTML page from public directory
			c.File(filepath.Join("public", "index.html"))
			c.Abort()
			return
		}

		if authHeader == bearer {
			c.Next()
		} else {
			// Serve HTML page from public directory
			c.File(filepath.Join("public", "index.html"))
			c.Abort()
			return
		}
	}
}

// ErrorHandler handles errors and sends appropriate responses
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Handle any errors that occurred during request processing
		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Something went wrong!",
				"details": err.Error(),
			})
		}
	}
}
