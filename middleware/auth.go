package middleware

import (
	"database/sql"
	"fdm-backend/config"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var db *sql.DB

// SetDB sets the database connection for middleware
func SetDB(database *sql.DB) {
	db = database
}

// AuthenticateToken validates the JWT token, checks session validity, and sets user info in context
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
			sessionID, _ := claims["sessionId"].(string)

			// Validate session - single device enforcement
			if db != nil && sessionID != "" {
				var isActive bool
				var expiresAt int64
				err := db.QueryRow(
					"SELECT isActive, expiresAt FROM Session WHERE id = ? AND userId = ?",
					sessionID, userID,
				).Scan(&isActive, &expiresAt)

				if err == sql.ErrNoRows {
					log.Printf("Session not found for user %s, session %s", userID, sessionID)
					c.JSON(http.StatusUnauthorized, gin.H{
						"error":   "Session expired or invalid",
						"code":    "SESSION_INVALID",
						"message": "You have been logged out. Please login again.",
					})
					c.Abort()
					return
				}

				if err != nil {
					log.Printf("Error checking session: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Error validating session"})
					c.Abort()
					return
				}

				// Check if session is still active
				if !isActive {
					c.JSON(http.StatusUnauthorized, gin.H{
						"error":   "Session has been terminated",
						"code":    "SESSION_TERMINATED",
						"message": "Your session was terminated because you logged in from another device.",
					})
					c.Abort()
					return
				}

				// Check if session has expired
				if time.Now().UnixMilli() > expiresAt {
					// Mark session as inactive
					db.Exec("UPDATE Session SET isActive = 0, updatedAt = ? WHERE id = ?", 
						time.Now().UnixMilli(), sessionID)
					c.JSON(http.StatusUnauthorized, gin.H{
						"error":   "Session expired",
						"code":    "SESSION_EXPIRED",
						"message": "Your session has expired. Please login again.",
					})
					c.Abort()
					return
				}
			}

			// Set user info in context
			c.Set("userId", userID)
			c.Set("userEmail", userEmail)
			c.Set("userRole", userRole)
			c.Set("sessionId", sessionID)

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
