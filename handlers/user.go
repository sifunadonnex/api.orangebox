package handlers

import (
	"database/sql"
	"fdm-backend/models"
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
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide an email and password"})
		return
	}

	// Find user by email
	var user models.User
	query := `SELECT id, email, role, fullName, designation, department, gateId, username, password, image, company, phone, createdAt, updatedAt FROM User WHERE email = ?`
	row := h.db.QueryRow(query, req.Email)
	
	err := row.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation, 
		&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image, 
		&user.Company, &user.Phone, &user.CreatedAt, &user.UpdatedAt)
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No User Found!!"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Verify password
	if user.Password == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid credentials"})
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(*user.Password), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid credentials"})
		return
	}

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
		err := rows.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
			&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
			&user.Company, &user.Phone, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning user"})
			return
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
	
	_, err = h.db.Exec(query, id, req.Email, req.Role, req.FullName, req.Username, string(hashedPassword), req.Company, now, now)
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
		_, err = h.db.Exec(query, string(hashedPassword), now, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
			return
		}

	case "profile":
		query := `UPDATE User SET fullName = ?, username = ?, email = ?, company = ?, phone = ?, department = ?, designation = ?, updatedAt = ? WHERE id = ?`
		_, err := h.db.Exec(query, req.FullName, req.Username, req.Email, req.Company, req.Phone, req.Department, req.Designation, now, id)
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
		_, err = h.db.Exec(query, req.FullName, req.Username, req.Email, req.Company, req.Phone, req.Role, string(hashedPassword), now, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid update level"})
		return
	}

	// Return updated user
	h.GetUserByID(c)
}

// GetUserByID retrieves a user by ID
func (h *UserHandler) GetUserByID(c *gin.Context) {
	id := c.Param("id")
	
	// Get user with aircraft
	query := `SELECT u.id, u.email, u.role, u.fullName, u.designation, u.department, u.gateId, u.username, u.password, u.image, u.company, u.phone, u.createdAt, u.updatedAt 
			  FROM User u WHERE u.id = ?`
	
	var user models.User
	row := h.db.QueryRow(query, id)
	err := row.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
		&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
		&user.Company, &user.Phone, &user.CreatedAt, &user.UpdatedAt)
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
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
			err := aircraftRows.Scan(&a.ID, &a.Airline, &a.AircraftMake, &a.ModelNumber, &a.SerialNumber, &a.UserID, &a.Parameters, &a.CreatedAt, &a.UpdatedAt)
			if err == nil {
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
		err := rows.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
			&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
			&user.Company, &user.Phone, &user.CreatedAt, &user.UpdatedAt)
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
				err := aircraftRows.Scan(&a.ID, &a.Airline, &a.AircraftMake, &a.ModelNumber, &a.SerialNumber, &a.UserID, &a.Parameters, &a.CreatedAt, &a.UpdatedAt)
				if err == nil {
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
	row := h.db.QueryRow(query, email)
	err := row.Scan(&user.ID, &user.Email, &user.Role, &user.FullName, &user.Designation,
		&user.Department, &user.GateID, &user.Username, &user.Password, &user.Image,
		&user.Company, &user.Phone, &user.CreatedAt, &user.UpdatedAt)
	
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
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
			err := aircraftRows.Scan(&a.ID, &a.Airline, &a.AircraftMake, &a.ModelNumber, &a.SerialNumber, &a.UserID, &a.Parameters, &a.CreatedAt, &a.UpdatedAt)
			if err == nil {
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
