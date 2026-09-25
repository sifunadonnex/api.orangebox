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
	companyHandler := handlers.NewCompanyHandler(db)
	subscriptionHandler := handlers.NewSubscriptionHandler(db)
	aircraftHandler := handlers.NewAircraftHandler(db)
	csvHandler := handlers.NewCSVHandler(db)
	eventHandler := handlers.NewEventHandler(db)
	exceedanceHandler := handlers.NewExceedanceHandler(db)
	reportHandler := handlers.NewReportHandler(db)
	notificationHandler := handlers.NewNotificationHandler(db)

	// Set database for auth middleware
	middleware.SetDB(db)

	// Public routes
	log.Println("Setting up routes...")
	router.POST("/login", userHandler.Login)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
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

	// Protected routes - all require authentication
	api := router.Group("/api")
	api.Use(middleware.AuthenticateToken())
	{
		// Company Management Routes (Admin Only)
		companies := api.Group("/companies")
		companies.Use(middleware.AdminOnly())
		{
			companies.POST("", companyHandler.CreateCompany)
			companies.GET("", companyHandler.GetCompanies)
			companies.PUT("/:id", companyHandler.UpdateCompany)
			companies.DELETE("/:id", companyHandler.DeleteCompany)
			companies.PUT("/:id/suspend", companyHandler.SuspendCompany)
			companies.PUT("/:id/activate", companyHandler.ActivateCompany)
		}
		// Company detail is restricted to the caller's company; platform admins
		// retain access for company administration.
		api.GET("/companies/:id", middleware.AnyAuthenticatedUser(), companyHandler.GetCompanyByID)

		// Subscription Management Routes (Admin Only)
		subscriptions := api.Group("/subscriptions")
		subscriptions.Use(middleware.AdminOnly())
		{
			subscriptions.POST("", subscriptionHandler.CreateSubscription)
			subscriptions.GET("", subscriptionHandler.GetSubscriptions)
			subscriptions.GET("/:id", subscriptionHandler.GetSubscriptionByID)
			subscriptions.PUT("/:id", subscriptionHandler.UpdateSubscription)
			subscriptions.DELETE("/:id", subscriptionHandler.DeleteSubscription)
			subscriptions.GET("/:id/status", subscriptionHandler.GetSubscriptionStatus)
			subscriptions.POST("/check-expired", subscriptionHandler.CheckExpiredSubscriptions)
		}

		// User Management Routes
		users := api.Group("/users")
		{
			users.GET("", middleware.GatekeeperOrAbove(), userHandler.GetUsers)
			users.POST("", middleware.GatekeeperOrAbove(), userHandler.CreateUser)
			users.GET("/email/:email", middleware.AnyAuthenticatedUser(), userHandler.GetUserByEmail)
			users.GET("/:id", middleware.AnyAuthenticatedUser(), userHandler.GetUserByID)
			users.PUT("/:id", middleware.GatekeeperOrAbove(), userHandler.UpdateUser)
			users.DELETE("/:id", middleware.AdminOrFDA(), userHandler.DeleteUser)
			users.PUT("/:id/activate", middleware.AdminOrFDA(), userHandler.ActivateUser)
			users.PUT("/:id/deactivate", middleware.AdminOrFDA(), userHandler.DeactivateUser)
			users.GET("/company/:companyId", middleware.GatekeeperOrAbove(), userHandler.GetUsersByCompanyID)
		}

		// Session Management Routes - Single Device Login Enforcement
		api.POST("/logout", userHandler.Logout)
		api.POST("/logout-all", userHandler.LogoutAllDevices)
		api.GET("/sessions", userHandler.GetActiveSessions)

		// Aircraft Management Routes (Company-scoped)
		aircrafts := api.Group("/aircrafts")
		{
			aircrafts.GET("", middleware.AnyAuthenticatedUser(), aircraftHandler.GetAircrafts)
			aircrafts.POST("", middleware.GatekeeperOrAbove(), aircraftHandler.CreateAircraft)
			aircrafts.GET("/company/:id", middleware.AnyAuthenticatedUser(), aircraftHandler.GetAircraftsByUserID)
			aircrafts.GET("/:id", middleware.AnyAuthenticatedUser(), aircraftHandler.GetAircraftByID)
			aircrafts.PUT("/:id", middleware.GatekeeperOrAbove(), aircraftHandler.UpdateAircraft)
			aircrafts.DELETE("/:id", middleware.AdminOrFDA(), aircraftHandler.DeleteAircraft)
		}

		// CSV/Flight Data Routes
		csvs := api.Group("/csv")
		{
			// Any authenticated operator user may submit a recording. UploadCSV is
			// responsible for enforcing aircraft/company ownership from the token.
			csvs.POST("", middleware.AnyAuthenticatedUser(), csvHandler.UploadCSV)
			csvs.GET("", middleware.AnyAuthenticatedUser(), csvHandler.GetCSVs)
			csvs.POST("/:id/analyze", middleware.GatekeeperOrAbove(), csvHandler.ReanalyzeCSV)
			csvs.GET("/:id/replay", middleware.AnyAuthenticatedUser(), csvHandler.GetFlightReplay)
			csvs.GET("/:id", middleware.AnyAuthenticatedUser(), csvHandler.DownloadCSV)
			csvs.DELETE("/:id", middleware.AdminOrFDA(), csvHandler.DeleteCSV)
		}
		api.GET("/flight/:id", middleware.AnyAuthenticatedUser(), csvHandler.GetCSVByID)
		recordings := api.Group("/recordings")
		{
			recordings.GET("/:id", middleware.AnyAuthenticatedUser(), csvHandler.GetRecordingReview)
			recordings.PUT("/:id/flights", middleware.GatekeeperOrAbove(), csvHandler.UpdateRecordingFlights)
			recordings.DELETE("/:id", middleware.AdminOrFDA(), csvHandler.DeleteRecording)
		}

		// Event Management Routes
		events := api.Group("/events")
		{
			events.GET("", middleware.AnyAuthenticatedUser(), eventHandler.GetEvents)
			events.POST("", middleware.GatekeeperOrAbove(), eventHandler.CreateEvent)
			events.POST("/validate", middleware.GatekeeperOrAbove(), eventHandler.ValidateEventPayload)
			events.GET("/:id", middleware.AnyAuthenticatedUser(), eventHandler.GetEventByID)
			events.PUT("/:id", middleware.AdminOrFDA(), eventHandler.UpdateEvent)
			events.POST("/:id/validate", middleware.AdminOrFDA(), eventHandler.ValidateEventVersion)
			events.POST("/:id/publish", middleware.AdminOrFDA(), eventHandler.PublishEventVersion)
			events.POST("/:id/backfill", middleware.AdminOrFDA(), csvHandler.BackfillEvent)
			events.DELETE("/:id", middleware.AdminOrFDA(), eventHandler.DeleteEvent)
		}

		// Exceedance Routes
		exceedances := api.Group("/exceedances")
		{
			exceedances.GET("", middleware.AnyAuthenticatedUser(), exceedanceHandler.GetExceedances)
			exceedances.GET("/benchmarks", middleware.AnyAuthenticatedUser(), exceedanceHandler.GetGlobalBenchmarks)
			exceedances.GET("/:id", middleware.AnyAuthenticatedUser(), exceedanceHandler.GetExceedanceByID)
			exceedances.GET("/flight/:id", middleware.AnyAuthenticatedUser(), exceedanceHandler.GetExceedancesByFlightID)
			exceedances.POST("", middleware.GatekeeperOrAbove(), exceedanceHandler.CreateExceedances)
			exceedances.PUT("/:id", middleware.AdminOrFDA(), exceedanceHandler.UpdateExceedance)
			exceedances.DELETE("/:id", middleware.AdminOrFDA(), exceedanceHandler.DeleteExceedance)
		}

		reports := api.Group("/reports")
		{
			reports.GET("/options", middleware.AnyAuthenticatedUser(), reportHandler.GetOptions)
			reports.GET("/overview", middleware.AnyAuthenticatedUser(), reportHandler.GetOverview)
			reports.GET("/events/aggregate", middleware.AnyAuthenticatedUser(), reportHandler.GetEventAggregate)
			reports.GET("/events/comparison", middleware.AnyAuthenticatedUser(), reportHandler.GetEventComparison)
			reports.GET("/events/benchmark", middleware.AnyAuthenticatedUser(), reportHandler.GetEventBenchmark)
			reports.GET("/events/location", middleware.AnyAuthenticatedUser(), reportHandler.GetEventLocation)
			reports.GET("/events/flights", middleware.AnyAuthenticatedUser(), reportHandler.GetEventfulFlights)
			reports.GET("/kpv/options", middleware.AnyAuthenticatedUser(), reportHandler.GetKPVOptions)
			reports.GET("/kpv/distribution", middleware.AnyAuthenticatedUser(), reportHandler.GetKPVDistribution)
		}

		// Notification Routes
		notifications := api.Group("/notifications")
		{
			notifications.POST("", middleware.GatekeeperOrAbove(), notificationHandler.CreateNotifications)
			notifications.GET("/user/:userId", middleware.AnyAuthenticatedUser(), notificationHandler.GetUserNotifications)
			notifications.PUT("/:id/read", middleware.AnyAuthenticatedUser(), notificationHandler.MarkNotificationAsRead)
			notifications.PUT("/user/:userId/mark-all-read", middleware.AnyAuthenticatedUser(), notificationHandler.MarkAllNotificationsAsRead)
		}
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
