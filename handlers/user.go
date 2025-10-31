package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	db *sql.DB
}

func NewUserHandler(db *sql.DB) *UserHandler {
	return &UserHandler{db: db}
}

// Login handles user authentication
func (h *UserHandler) Login(c *gin.Context) {
	log.Println("Login attempt started")
	
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Error binding JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide an email and password"})
		return
	}
	
	log.Printf("Login attempt for email: %s", req.Email)

	// Find user by email - handle SQLite datetime as int64
	var user models.User
	var role, fullName, designation, department, gateId, username, password, image, company, phone sql.NullString
	var createdAtUnix, updatedAtUnix sql.NullInt64

	query := `SELECT id, email, role, fullName, designation, department, gateId, username, password, image, company, phone, createdAt, updatedAt FROM User WHERE email = ?`
	log.Printf("Executing query: %s with email: %s", query, req.Email)
	
	row := h.db.QueryRow(query, req.Email)
	
	err := row.Scan(&user.ID, &user.Email, &role, &fullName, &designation, 
		&department, &gateId, &username, &password, &image, 
		&company, &phone, &createdAtUnix, &updatedAtUnix)
	
	if err == sql.ErrNoRows {
		log.Printf("No user found with email: %s", req.Email)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No User Found!!"})
		return
	}
	if err != nil {
		// More detailed error logging
		log.Printf("Database scan error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}
	
	log.Printf("User found: %s", user.Email)

	// Map nullable fields to pointers
	if role.Valid {
		user.Role = &role.String
	}
	if fullName.Valid {
		user.FullName = &fullName.String
	}
	if designation.Valid {
		user.Designation = &designation.String
	}
	if department.Valid {
		user.Department = &department.String
	}
	if gateId.Valid {
		user.GateID = &gateId.String
	}
	if username.Valid {
		user.Username = &username.String
	}
	if password.Valid {
		user.Password = &password.String
	}
	if image.Valid {
		user.Image = &image.String
	}
	if company.Valid {
		user.Company = &company.String
	}
	if phone.Valid {
		user.Phone = &phone.String
	}
	if createdAtUnix.Valid {
		user.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0) // Convert milliseconds to seconds
	}
	if updatedAtUnix.Valid {
		user.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0) // Convert milliseconds to seconds
	}

	// Verify password
	if user.Password == nil {
		log.Println("User has no password set")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid credentials"})
		return
	}

	log.Println("Verifying password...")
	err = bcrypt.CompareHashAndPassword([]byte(*user.Password), []byte(req.Password))
	if err != nil {
		log.Printf("Password verification failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid credentials"})
		return
	}

	log.Println("Login successful")
	// Remove sensitive information and prepare response
	user.Password = nil
	// Add name field for compatibility
	response := struct {
		models.User
		Name *string `json:"name"`
	}{
		User: user,
		Name: user.FullName,
	}

	c.JSON(http.StatusOK, gin.H{"user": response})
}

// GetUsers retrieves all users
func (h *UserHandler) GetUsers(c *gin.Context) {
	query := `SELECT id, email, role, fullName, designation, department, gateId, username, password, image, company, phone, createdAt, updatedAt FROM User`
	rows, err := h.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		var createdAtUnix, updatedAtUnix sql.NullInt64
		
		err := rows.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
			&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
			&user.Company, &user.Phone, &createdAtUnix, &updatedAtUnix)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning user"})
			return
		}
		
		if createdAtUnix.Valid {
			user.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			user.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
		}
		
		user.Password = nil // Remove password from response
		users = append(users, user)
	}

	c.JSON(http.StatusOK, users)
}

// CreateUser creates a new user
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error hashing password"})
		return
	}

	// Generate ID and timestamps
	id := uuid.New().String()
	now := time.Now()

	// Insert user
	query := `INSERT INTO User (id, email, role, fullName, username, password, company, createdAt, updatedAt) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	_, err = h.db.Exec(query, id, req.Email, req.Role, req.FullName, req.Username, string(hashedPassword), req.Company, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating user"})
		return
	}

	// Return created user (without password)
	user := models.User{
		ID:        id,
		Email:     req.Email,
		Role:      &req.Role,
		FullName:  &req.FullName,
		Username:  &req.Username,
		Company:   &req.Company,
		CreatedAt: now,
		UpdatedAt: now,
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUser updates an existing user
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()

	switch req.UpdateLevel {
	case "password":
		if req.Password == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password is required"})
			return
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error hashing password"})
			return
		}
		query := `UPDATE User SET password = ?, updatedAt = ? WHERE id = ?`
		_, err = h.db.Exec(query, string(hashedPassword), now.UnixMilli(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
			return
		}

	case "profile":
		query := `UPDATE User SET fullName = ?, username = ?, email = ?, company = ?, phone = ?, department = ?, designation = ?, updatedAt = ? WHERE id = ?`
		_, err := h.db.Exec(query, req.FullName, req.Username, req.Email, req.Company, req.Phone, req.Department, req.Designation, now.UnixMilli(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
			return
		}

	case "admin":
		if req.Password == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password is required for admin update"})
			return
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error hashing password"})
			return
		}
		query := `UPDATE User SET fullName = ?, username = ?, email = ?, company = ?, phone = ?, role = ?, password = ?, updatedAt = ? WHERE id = ?`
		_, err = h.db.Exec(query, req.FullName, req.Username, req.Email, req.Company, req.Phone, req.Role, string(hashedPassword), now.UnixMilli(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid update level"})
		return
	}

	// Fetch and return updated user
	query := `SELECT u.id, u.email, u.role, u.fullName, u.designation, u.department, u.gateId, u.username, u.password, u.image, u.company, u.phone, u.createdAt, u.updatedAt 
			  FROM User u WHERE u.id = ?`
	
	var user models.User
	var createdAtUnix, updatedAtUnix sql.NullInt64
	row := h.db.QueryRow(query, id)
	err := row.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
		&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
		&user.Company, &user.Phone, &createdAtUnix, &updatedAtUnix)
	
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching user"})
		return
	}

	// Convert Unix timestamps to time.Time
	if createdAtUnix.Valid {
		user.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
	}
	if updatedAtUnix.Valid {
		user.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
	}

	// Don't return password
	user.Password = nil

	c.JSON(http.StatusOK, user)
}

// GetUserByID retrieves a user by ID
func (h *UserHandler) GetUserByID(c *gin.Context) {
	id := c.Param("id")
	
	// Get user with aircraft
	query := `SELECT u.id, u.email, u.role, u.fullName, u.designation, u.department, u.gateId, u.username, u.password, u.image, u.company, u.phone, u.createdAt, u.updatedAt 
			  FROM User u WHERE u.id = ?`
	
	var user models.User
	var createdAtUnix, updatedAtUnix sql.NullInt64
	row := h.db.QueryRow(query, id)
	err := row.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
		&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
		&user.Company, &user.Phone, &createdAtUnix, &updatedAtUnix)
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Handle datetime conversion
	if createdAtUnix.Valid {
		user.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
	}
	if updatedAtUnix.Valid {
		user.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
	}

	user.Password = nil // Remove password from response

	// Get user's aircraft
	aircraftQuery := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, userId, parameters, createdAt, updatedAt FROM Aircraft WHERE userId = ?`
	aircraftRows, err := h.db.Query(aircraftQuery, id)
	if err == nil {
		defer aircraftRows.Close()
		var aircraft []models.Aircraft
		for aircraftRows.Next() {
			var a models.Aircraft
			var modelNumber, parameters sql.NullString
			var aircraftCreatedAtUnix, aircraftUpdatedAtUnix sql.NullInt64
			
			err := aircraftRows.Scan(&a.ID, &a.Airline, &a.AircraftMake, &modelNumber, &a.SerialNumber, &a.UserID, &parameters, &aircraftCreatedAtUnix, &aircraftUpdatedAtUnix)
			if err == nil {
				// Handle nullable fields
				if modelNumber.Valid {
					a.ModelNumber = &modelNumber.String
				}
				if parameters.Valid {
					a.Parameters = &parameters.String
				}
				if aircraftCreatedAtUnix.Valid {
					a.CreatedAt = time.Unix(aircraftCreatedAtUnix.Int64/1000, 0)
				}
				if aircraftUpdatedAtUnix.Valid {
					a.UpdatedAt = time.Unix(aircraftUpdatedAtUnix.Int64/1000, 0)
				}
				aircraft = append(aircraft, a)
			}
		}
		
		// Create response with aircraft
		response := struct {
			models.User
			Aircraft []models.Aircraft `json:"Aircraft"`
		}{
			User:     user,
			Aircraft: aircraft,
		}
		c.JSON(http.StatusOK, response)
		return
	}

	c.JSON(http.StatusOK, user)
}

// GetUsersByGateID retrieves users by gate ID
func (h *UserHandler) GetUsersByGateID(c *gin.Context) {
	gateID := c.Param("id")
	
	query := `SELECT u.id, u.email, u.role, u.fullName, u.designation, u.department, u.gateId, u.username, u.password, u.image, u.company, u.phone, u.createdAt, u.updatedAt 
			  FROM User u WHERE u.gateId = ?`
	
	rows, err := h.db.Query(query, gateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	var users []interface{}
	for rows.Next() {
		var user models.User
		var createdAtUnix, updatedAtUnix sql.NullInt64
		
		err := rows.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
			&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
			&user.Company, &user.Phone, &createdAtUnix, &updatedAtUnix)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning user"})
			return
		}

		// Handle datetime conversion
		if createdAtUnix.Valid {
			user.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
		}
		if updatedAtUnix.Valid {
			user.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
		}
		if err != nil {
			continue
		}
		user.Password = nil

		// Get user's aircraft
		aircraftQuery := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, userId, parameters, createdAt, updatedAt FROM Aircraft WHERE userId = ?`
		aircraftRows, err := h.db.Query(aircraftQuery, user.ID)
		if err == nil {
			defer aircraftRows.Close()
			var aircraft []models.Aircraft
			for aircraftRows.Next() {
				var a models.Aircraft
				var modelNumber, parameters sql.NullString
				var aircraftCreatedAtUnix, aircraftUpdatedAtUnix sql.NullInt64
				
				err := aircraftRows.Scan(&a.ID, &a.Airline, &a.AircraftMake, &modelNumber, &a.SerialNumber, &a.UserID, &parameters, &aircraftCreatedAtUnix, &aircraftUpdatedAtUnix)
				if err == nil {
					// Handle nullable fields
					if modelNumber.Valid {
						a.ModelNumber = &modelNumber.String
					}
					if parameters.Valid {
						a.Parameters = &parameters.String
					}
					if aircraftCreatedAtUnix.Valid {
						a.CreatedAt = time.Unix(aircraftCreatedAtUnix.Int64/1000, 0)
					}
					if aircraftUpdatedAtUnix.Valid {
						a.UpdatedAt = time.Unix(aircraftUpdatedAtUnix.Int64/1000, 0)
					}
					aircraft = append(aircraft, a)
				}
			}
			
			userWithAircraft := struct {
				models.User
				Aircraft []models.Aircraft `json:"Aircraft"`
			}{
				User:     user,
				Aircraft: aircraft,
			}
			users = append(users, userWithAircraft)
		} else {
			users = append(users, user)
		}
	}

	c.JSON(http.StatusOK, users)
}

// GetUserByEmail retrieves a user by email
func (h *UserHandler) GetUserByEmail(c *gin.Context) {
	email := c.Param("id") // Note: param is "id" but it's actually email
	
	query := `SELECT u.id, u.email, u.role, u.fullName, u.designation, u.department, u.gateId, u.username, u.password, u.image, u.company, u.phone, u.createdAt, u.updatedAt 
			  FROM User u WHERE u.email = ?`
	
	var user models.User
	var createdAtUnix, updatedAtUnix sql.NullInt64
	row := h.db.QueryRow(query, email)
	err := row.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
		&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
		&user.Company, &user.Phone, &createdAtUnix, &updatedAtUnix)
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Handle datetime conversion
	if createdAtUnix.Valid {
		user.CreatedAt = time.Unix(createdAtUnix.Int64/1000, 0)
	}
	if updatedAtUnix.Valid {
		user.UpdatedAt = time.Unix(updatedAtUnix.Int64/1000, 0)
	}

	user.Password = nil

	// Get user's aircraft
	aircraftQuery := `SELECT id, airline, aircraftMake, modelNumber, serialNumber, userId, parameters, createdAt, updatedAt FROM Aircraft WHERE userId = ?`
	aircraftRows, err := h.db.Query(aircraftQuery, user.ID)
	if err == nil {
		defer aircraftRows.Close()
		var aircraft []models.Aircraft
		for aircraftRows.Next() {
			var a models.Aircraft
			var modelNumber, parameters sql.NullString
			var aircraftCreatedAtUnix, aircraftUpdatedAtUnix sql.NullInt64
			
			err := aircraftRows.Scan(&a.ID, &a.Airline, &a.AircraftMake, &modelNumber, &a.SerialNumber, &a.UserID, &parameters, &aircraftCreatedAtUnix, &aircraftUpdatedAtUnix)
			if err == nil {
				// Handle nullable fields
				if modelNumber.Valid {
					a.ModelNumber = &modelNumber.String
				}
				if parameters.Valid {
					a.Parameters = &parameters.String
				}
				if aircraftCreatedAtUnix.Valid {
					a.CreatedAt = time.Unix(aircraftCreatedAtUnix.Int64/1000, 0)
				}
				if aircraftUpdatedAtUnix.Valid {
					a.UpdatedAt = time.Unix(aircraftUpdatedAtUnix.Int64/1000, 0)
				}
				aircraft = append(aircraft, a)
			}
		}
		
		response := struct {
			models.User
			Aircraft []models.Aircraft `json:"Aircraft"`
		}{
			User:     user,
			Aircraft: aircraft,
		}
		c.JSON(http.StatusOK, response)
		return
	}

	c.JSON(http.StatusOK, user)
}

// DeleteUser deletes a user
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	
	query := `DELETE FROM User WHERE id = ?`
	result, err := h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting user"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}
