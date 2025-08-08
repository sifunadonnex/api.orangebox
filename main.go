package main

import (
	"fdm-backend/config"
	"fdm-backend/database"
	"fdm-backend/handlers"
	"fdm-backend/middleware"
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize database
	db, err := database.InitDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Create Gin router
	router := gin.Default()

	// CORS configuration
	corsConfig := cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}
	router.Use(cors.New(corsConfig))

	// File upload configuration
	router.MaxMultipartMemory = 8 << 20 // 8 MiB

	// Static file serving
	router.Static("/csvs", "./csvs")

	// Error handling middleware
	router.Use(middleware.ErrorHandler())

	// Initialize handlers
	userHandler := handlers.NewUserHandler(db)
	aircraftHandler := handlers.NewAircraftHandler(db)
	csvHandler := handlers.NewCSVHandler(db)
	eventHandler := handlers.NewEventHandler(db)
	exceedanceHandler := handlers.NewExceedanceHandler(db)

	// Public routes
	router.POST("/login", userHandler.Login)
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Hello World!"})
	})

	// Protected routes
	protected := router.Group("/")
	protected.Use(middleware.AuthenticateToken())
	{
		// User routes
		protected.GET("/users", userHandler.GetUsers)
		protected.POST("/users", userHandler.CreateUser)
		protected.PUT("/users/:id", userHandler.UpdateUser)
		protected.GET("/users/:id", userHandler.GetUserByID)
		protected.GET("/users/gate/:id", userHandler.GetUsersByGateID)
		protected.GET("/user/:id", userHandler.GetUserByEmail)
		protected.DELETE("/users/:id", userHandler.DeleteUser)

		// Aircraft routes
		protected.GET("/aircrafts", aircraftHandler.GetAircrafts)
		protected.GET("/aircrafts/:id", aircraftHandler.GetAircraftsByUserID)
		protected.POST("/aircrafts", aircraftHandler.CreateAircraft)
		protected.PUT("/aircrafts/:id", aircraftHandler.UpdateAircraft)
		protected.DELETE("/aircrafts/:id", aircraftHandler.DeleteAircraft)

		// CSV routes
		protected.POST("/csv", csvHandler.UploadCSV)
		protected.GET("/csv", csvHandler.GetCSVs)
		protected.GET("/csv/:id", csvHandler.DownloadCSV)
		protected.GET("/flight/:id", csvHandler.GetCSVByID)
		protected.DELETE("/csv/:id", csvHandler.DeleteCSV)

		// Event routes
		protected.POST("/events", eventHandler.CreateEvent)
		protected.GET("/events", eventHandler.GetEvents)
		protected.GET("/events/:id", eventHandler.GetEventByID)
		protected.PUT("/events/:id", eventHandler.UpdateEvent)
		protected.DELETE("/events/:id", eventHandler.DeleteEvent)

		// Exceedance routes
		protected.GET("/exceedances", exceedanceHandler.GetExceedances)
		protected.GET("/exceedances/:id", exceedanceHandler.GetExceedanceByID)
		protected.GET("/exceedances/flight/:id", exceedanceHandler.GetExceedancesByFlightID)
		protected.POST("/exceedances", exceedanceHandler.CreateExceedances)
		protected.PUT("/exceedances/:id", exceedanceHandler.UpdateExceedance)
		protected.DELETE("/exceedances/:id", exceedanceHandler.DeleteExceedance)
	}

	// Start server
	port := config.GetPort()
	log.Printf("Server starting on http://localhost:%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
