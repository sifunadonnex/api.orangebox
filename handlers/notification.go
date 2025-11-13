package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type NotificationHandler struct {
	db *sql.DB
}

func NewNotificationHandler(db *sql.DB) *NotificationHandler {
	return &NotificationHandler{db: db}
}

// CreateNotifications creates notifications for users based on exceedance data
func (h *NotificationHandler) CreateNotifications(c *gin.Context) {
	var req models.CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get all users who should be notified (e.g., admins, operators)
	query := `SELECT id FROM User WHERE role IN ('admin', 'operator')`
	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching users"})
		return
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}

	// Create notifications for each exceedance and user
	now := time.Now()
	var createdNotifications []models.Notification

	for _, exc := range req.Exceedances {
		// Create a message for the exceedance
		message := createNotificationMessage(exc.Description, exc.Level, exc.Phase, exc.Parameter)

		for _, userID := range userIDs {
			id := uuid.New().String()
			notification := models.Notification{
				ID:           id,
				UserID:       userID,
				ExceedanceID: req.FlightID, // This might need to be adjusted based on your data structure
				Message:      message,
				Level:        exc.Level,
				IsRead:       false,
				CreatedAt:    now,
				UpdatedAt:    now,
			}

			query := `INSERT INTO Notification (id, userId, exceedanceId, message, level, isRead, createdAt, updatedAt)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

			_, err := h.db.Exec(query, notification.ID, notification.UserID,
				notification.ExceedanceID, notification.Message, notification.Level,
				notification.IsRead, notification.CreatedAt.UnixMilli(),
				notification.UpdatedAt.UnixMilli())

			if err != nil {
				continue
			}

			createdNotifications = append(createdNotifications, notification)
		}
	}

	c.JSON(http.StatusOK, createdNotifications)
}

// GetUserNotifications retrieves notifications for a specific user
func (h *NotificationHandler) GetUserNotifications(c *gin.Context) {
	userID := c.Param("userId")

	query := `SELECT n.id, n.userId, n.exceedanceId, n.message, n.level, n.isRead, n.createdAt, n.updatedAt
			  FROM Notification n
			  WHERE n.userId = ?
			  ORDER BY n.createdAt DESC`

	rows, err := h.db.Query(query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var notifications []models.Notification
	for rows.Next() {
		var notification models.Notification
		var createdAt, updatedAt sql.NullTime

		err := rows.Scan(&notification.ID, &notification.UserID,
			&notification.ExceedanceID, &notification.Message,
			&notification.Level, &notification.IsRead,
			&createdAt, &updatedAt)

		if err != nil {
			continue
		}

		if createdAt.Valid {
			notification.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			notification.UpdatedAt = updatedAt.Time
		}
		notifications = append(notifications, notification)
	}

	c.JSON(http.StatusOK, notifications)
}

// MarkNotificationAsRead marks a notification as read
func (h *NotificationHandler) MarkNotificationAsRead(c *gin.Context) {
	id := c.Param("id")
	now := time.Now()

	query := `UPDATE Notification SET isRead = true, updatedAt = ? WHERE id = ?`
	result, err := h.db.Exec(query, now.UnixMilli(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating notification"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification marked as read"})
}

// MarkAllNotificationsAsRead marks all notifications as read for a user
func (h *NotificationHandler) MarkAllNotificationsAsRead(c *gin.Context) {
	userID := c.Param("userId")
	now := time.Now()

	query := `UPDATE Notification SET isRead = true, updatedAt = ? WHERE userId = ? AND isRead = false`
	result, err := h.db.Exec(query, now.UnixMilli(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating notifications"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	c.JSON(http.StatusOK, gin.H{"message": "All notifications marked as read", "updated": rowsAffected})
}

// Helper function to create notification messages
func createNotificationMessage(description, level, phase, parameter string) string {
	return description + " detected during " + phase + " phase. Parameter: " + parameter + " (Level " + level + ")"
}
