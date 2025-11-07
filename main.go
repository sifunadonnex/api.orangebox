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
	log.Println("Initializing database connection...")
	db, err := database.InitDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Run database migrations
	log.Println("Running database migrations...")
	if err := database.RunMigrations(db); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}
	log.Println("Database migrations completed successfully")

	// Create Gin router
	log.Println("Setting up Gin router...")
	router := gin.Default()

	// CORS configuration
	corsConfig := cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "https://www.orangebox.co.ke", "http://www.orangebox.co.ke"},
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
	log.Println("Initializing handlers...")
	userHandler := handlers.NewUserHandler(db)
	aircraftHandler := handlers.NewAircraftHandler(db)
	csvHandler := handlers.NewCSVHandler(db)
	eventHandler := handlers.NewEventHandler(db)
	exceedanceHandler := handlers.NewExceedanceHandler(db)
	notificationHandler := handlers.NewNotificationHandler(db)

	// Public routes
	log.Println("Setting up routes...")
	router.POST("/login", userHandler.Login)
	router.GET("/test-simple", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Server is working"})
	})
	router.GET("/test-db", func(c *gin.Context) {
		// Test database connection and query
		query := `SELECT COUNT(*) as count FROM User`
		var count int
		err := db.QueryRow(query).Scan(&count)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database test failed", "details": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Database connected", "user_count": count})
	})
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

		// Notification routes
		protected.POST("/notifications", notificationHandler.CreateNotifications)
		protected.GET("/notifications/user/:userId", notificationHandler.GetUserNotifications)
		protected.PUT("/notifications/:id/read", notificationHandler.MarkNotificationAsRead)
		protected.PUT("/notifications/user/:userId/mark-all-read", notificationHandler.MarkAllNotificationsAsRead)
	}

	// Catch-all for unmatched routes (for debugging)
	router.NoRoute(func(c *gin.Context) {
		log.Printf("NoRoute: Method=%s Path=%s", c.Request.Method, c.Request.URL.Path)
		c.JSON(http.StatusNotFound, gin.H{"error": "Route not found", "path": c.Request.URL.Path, "method": c.Request.Method})
	})

	// Start server
	port := config.GetPort()
	log.Printf("Server starting on http://localhost:%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
