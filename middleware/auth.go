package middleware

import (
	"database/sql"
	"fdm-backend/config"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var db *sql.DB

// SetDB sets the database connection for middleware
func SetDB(database *sql.DB) {
	db = database
}

// AuthenticateToken validates the JWT token and sets user info in context
func AuthenticateToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Check if it's a Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format. Use: Bearer <token>"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Parse and validate JWT token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Verify signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(config.GetJWTSecret()), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Extract claims
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			userID, _ := claims["userId"].(string)
			userEmail, _ := claims["email"].(string)
			userRole, _ := claims["role"].(string)

			// Set user info in context
			c.Set("userId", userID)
			c.Set("userEmail", userEmail)
			c.Set("userRole", userRole)

			// Fetch additional user info from database if needed
			if db != nil {
				var companyID *string
				var isActive bool
				err := db.QueryRow("SELECT companyId, isActive FROM User WHERE id = ?", userID).Scan(&companyID, &isActive)
				if err == nil {
					if !isActive {
						c.JSON(http.StatusForbidden, gin.H{"error": "Account is deactivated"})
						c.Abort()
						return
					}

					if companyID != nil {
						c.Set("userCompanyId", *companyID)

						// Check if company is active
						var companyStatus string
						db.QueryRow("SELECT status FROM Company WHERE id = ?", *companyID).Scan(&companyStatus)
						if companyStatus == "suspended" || companyStatus == "expired" {
							c.JSON(http.StatusForbidden, gin.H{
								"error":   "Company account is " + companyStatus,
								"message": "Please contact support to reactivate your account",
								"status":  companyStatus,
							})
							c.Abort()
							return
						}

						c.Set("companyStatus", companyStatus)
					}
				}
			}

			c.Next()
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
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
				"error":   "Something went wrong!",
				"details": err.Error(),
			})
		}
	}
}
