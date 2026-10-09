package handlers

import (
	"fdm-backend/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes keeps event-definition workflow permissions consistent.
// The handlers enforce company scope on every read and mutation.
func (h *EventHandler) RegisterRoutes(events *gin.RouterGroup) {
	events.GET("", middleware.AnyAuthenticatedUser(), h.GetEvents)
	events.POST("", middleware.GatekeeperOrAbove(), h.CreateEvent)
	events.POST("/validate", middleware.GatekeeperOrAbove(), h.ValidateEventPayload)
	events.GET("/:id", middleware.AnyAuthenticatedUser(), h.GetEventByID)
	events.PUT("/:id", middleware.GatekeeperOrAbove(), h.UpdateEvent)
	events.POST("/:id/validate", middleware.GatekeeperOrAbove(), h.ValidateEventVersion)
	events.POST("/:id/publish", middleware.GatekeeperOrAbove(), h.PublishEventVersion)
	events.DELETE("/:id", middleware.AdminOrFDA(), h.DeleteEvent)
}
