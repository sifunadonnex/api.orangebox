package main

import (
	"fdm-backend/database"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	log.Println("Starting database seeding...")

	// Initialize database
	db, err := database.InitDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Create admin user
	adminID := uuid.New().String()
	adminEmail := "admin@fdm.com"
	adminPassword := "Admin@123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("Failed to hash password:", err)
	}

	now := time.Now()
	fullName := "System Administrator"
	username := "admin"

	// Check if admin already exists
	var existingID string
	err = db.QueryRow("SELECT id FROM User WHERE email = ?", adminEmail).Scan(&existingID)
	if err == nil {
		log.Println("Admin user already exists, skipping...")
	} else {
		// Create admin user without company
		_, err = db.Exec(`
			INSERT INTO User (
				id, email, role, fullName, username, password, isActive, createdAt, updatedAt
			) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
		`, adminID, adminEmail, "admin", fullName, username, string(hashedPassword), now, now)

		if err != nil {
			log.Fatal("Failed to create admin user:", err)
		}
		log.Printf("✅ Admin user created: %s / %s", adminEmail, adminPassword)
	}

	// Create test subscription
	subscriptionID := uuid.New().String()
	subscriptionName := "Professional Plan"

	var existingSubID string
	err = db.QueryRow("SELECT id FROM Subscription WHERE planName = ?", subscriptionName).Scan(&existingSubID)
	if err == nil {
		log.Println("Test subscription already exists, skipping...")
	} else {
		_, err = db.Exec(`
			INSERT INTO Subscription (
				id, planName, planType, maxUsers, maxAircraft, maxFlightsPerMonth, 
				maxStorageGB, price, startDate, endDate, isActive, autoRenew, createdAt, updatedAt
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 1, ?, ?)
		`, subscriptionID, subscriptionName, "professional", 50, 20, 1000, 500.0, 999.99,
			now, now.AddDate(1, 0, 0), now, now)

		if err != nil {
			log.Fatal("Failed to create subscription:", err)
		}
		log.Printf("✅ Test subscription created: %s", subscriptionName)
	}

	// Create test company
	companyID := uuid.New().String()
	companyName := "Demo Aviation"
	companyEmail := "contact@demoaviation.com"
	companyPhone := "+1-555-0100"
	companyAddress := "123 Airport Road"
	companyCountry := "United States"

	var existingCompanyID string
	err = db.QueryRow("SELECT id FROM Company WHERE email = ?", companyEmail).Scan(&existingCompanyID)
	if err == nil {
		log.Println("Test company already exists, skipping...")
		companyID = existingCompanyID
	} else {
		_, err = db.Exec(`
			INSERT INTO Company (
				id, name, email, phone, address, country, status, subscriptionId, createdAt, updatedAt
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, companyID, companyName, companyEmail, companyPhone, companyAddress,
			companyCountry, "active", subscriptionID, now, now)

		if err != nil {
			log.Fatal("Failed to create company:", err)
		}
		log.Printf("✅ Test company created: %s", companyName)
	}

	// Create test users with different roles
	testUsers := []struct {
		email    string
		password string
		role     string
		fullName string
		username string
	}{
		{"fda@demoaviation.com", "FDA@123", "fda", "John FDA", "fda"},
		{"gatekeeper@demoaviation.com", "Gate@123", "gatekeeper", "Jane Gatekeeper", "gatekeeper"},
		{"user@demoaviation.com", "User@123", "user", "Bob User", "user"},
	}

	for _, u := range testUsers {
		var existingUserID string
		err = db.QueryRow("SELECT id FROM User WHERE email = ?", u.email).Scan(&existingUserID)
		if err == nil {
			log.Printf("User %s already exists, skipping...", u.email)
			continue
		}

		userID := uuid.New().String()
		hashedPwd, _ := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)

		_, err = db.Exec(`
			INSERT INTO User (
				id, email, role, fullName, username, password, isActive, companyId, createdAt, updatedAt
			) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
		`, userID, u.email, u.role, u.fullName, u.username, string(hashedPwd), companyID, now, now)

		if err != nil {
			log.Printf("Failed to create user %s: %v", u.email, err)
			continue
		}
		log.Printf("✅ Test user created: %s / %s (Role: %s)", u.email, u.password, u.role)
	}

	// Create test aircraft
	aircraftID := uuid.New().String()
	aircraftRegistration := "N123AB"

	var existingAircraftID string
	err = db.QueryRow("SELECT id FROM Aircraft WHERE registration = ?", aircraftRegistration).Scan(&existingAircraftID)
	if err == nil {
		log.Println("Test aircraft already exists, skipping...")
	} else {
		_, err = db.Exec(`
			INSERT INTO Aircraft (
				id, airline, aircraftMake, modelNumber, serialNumber, registration, companyId, createdAt, updatedAt
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, aircraftID, "Demo Aviation", "Boeing", "737-800", "SN12345", aircraftRegistration, companyID, now, now)

		if err != nil {
			log.Printf("Failed to create aircraft: %v", err)
		} else {
			log.Printf("✅ Test aircraft created: %s", aircraftRegistration)
		}
	}

	log.Println("\n========================================")
	log.Println("Database seeding completed successfully!")
	log.Println("========================================")
	log.Println("\nTest Accounts:")
	log.Println("1. Admin (Full Access):")
	log.Println("   Email: admin@fdm.com")
	log.Println("   Password: Admin@123")
	log.Println("\n2. FDA User (Validation & Analysis):")
	log.Println("   Email: fda@demoaviation.com")
	log.Println("   Password: FDA@123")
	log.Println("\n3. Gatekeeper (Add Events & View):")
	log.Println("   Email: gatekeeper@demoaviation.com")
	log.Println("   Password: Gate@123")
	log.Println("\n4. Regular User (View Only):")
	log.Println("   Email: user@demoaviation.com")
	log.Println("   Password: User@123")
	log.Println("\nCompany: Demo Aviation")
	log.Println("Subscription: Professional Plan (50 users, 20 aircraft)")
	log.Println("========================================")
}
