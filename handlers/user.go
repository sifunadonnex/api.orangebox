package handlers

import (
	"database/sql"
	"fdm-backend/models"
	"fdm-backend/utils"
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

// Login handles user authentication with JWT
func (h *UserHandler) Login(c *gin.Context) {
	log.Println("Login attempt started")

	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Error binding JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide an email and password"})
		return
	}

	log.Printf("Login attempt for email: %s", req.Email)

	// Find user by email with new schema
	var user models.User
	var fullName, designation, department, username, password, image, phone, companyID, lastLoginAt sql.NullString
	var isActive bool
	var createdAt, updatedAt time.Time

	query := `
		SELECT id, email, role, fullName, designation, department, username, password, image, 
		       phone, isActive, companyId, lastLoginAt, createdAt, updatedAt 
		FROM User WHERE email = ?
	`

	err := h.db.QueryRow(query, req.Email).Scan(
		&user.ID, &user.Email, &user.Role, &fullName, &designation,
		&department, &username, &password, &image, &phone, &isActive,
		&companyID, &lastLoginAt, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		log.Printf("No user found with email: %s", req.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}
	if err != nil {
		log.Printf("Database scan error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}

	log.Printf("User found: %s with role: %s", user.Email, user.Role)

	// Check if user is active
	if !isActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "Account is deactivated"})
		return
	}

	// Map nullable fields
	if fullName.Valid {
		user.FullName = &fullName.String
	}
	if designation.Valid {
		user.Designation = &designation.String
	}
	if department.Valid {
		user.Department = &department.String
	}
	if username.Valid {
		user.Username = &username.String
	}
	if image.Valid {
		user.Image = &image.String
	}
	if phone.Valid {
		user.Phone = &phone.String
	}
	if companyID.Valid {
		user.CompanyID = &companyID.String
	}
	user.IsActive = isActive
	user.CreatedAt = createdAt
	user.UpdatedAt = updatedAt

	// Verify password
	if !password.Valid {
		log.Println("User has no password set")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	log.Println("Verifying password...")
	err = bcrypt.CompareHashAndPassword([]byte(password.String), []byte(req.Password))
	if err != nil {
		log.Printf("Password verification failed: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Check company status if user belongs to a company
	if user.CompanyID != nil {
		var companyStatus string
		err = h.db.QueryRow("SELECT status FROM Company WHERE id = ?", *user.CompanyID).Scan(&companyStatus)
		if err == nil {
			if companyStatus == "suspended" || companyStatus == "expired" {
				c.JSON(http.StatusForbidden, gin.H{
					"error":   "Company account is " + companyStatus,
					"message": "Please contact support to reactivate your account",
				})
				return
			}
		}
	}

	// Generate JWT token
	token, err := utils.GenerateJWT(user.ID, user.Email, user.Role, user.CompanyID)
	if err != nil {
		log.Printf("Error generating JWT: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error generating token"})
		return
	}

	// Update last login
	now := time.Now()
	h.db.Exec("UPDATE User SET lastLoginAt = ? WHERE id = ?", now, user.ID)
	user.LastLoginAt = &now

	// Fetch company details if exists
	if user.CompanyID != nil {
		company := &models.Company{}
		err = h.db.QueryRow(`
			SELECT id, name, email, phone, address, country, logo, status, subscriptionId, createdAt, updatedAt 
			FROM Company WHERE id = ?
		`, *user.CompanyID).Scan(
			&company.ID, &company.Name, &company.Email, &company.Phone, &company.Address,
			&company.Country, &company.Logo, &company.Status, &company.SubscriptionID,
			&company.CreatedAt, &company.UpdatedAt,
		)
		if err == nil {
			user.Company = company
		}
	}

	log.Println("Login successful")

	// Return user with token (no password)
	c.JSON(http.StatusOK, gin.H{
		"user":    user,
		"token":   token,
		"message": "Login successful",
	})
}

// GetUsers retrieves all users with their company information
func (h *UserHandler) GetUsers(c *gin.Context) {
	// Get requesting user's role and company
	userRole, _ := c.Get("userRole")
	userCompanyID, companyExists := c.Get("userCompanyId")

	query := `
		SELECT 
			u.id, u.email, u.role, u.fullName, u.designation, u.department, u.username, u.image, 
			u.phone, u.isActive, u.companyId, u.lastLoginAt, u.createdAt, u.updatedAt,
			c.id, c.name, c.email, c.phone, c.address, c.country, 
			c.logo, c.status, c.subscriptionId, c.createdAt, c.updatedAt
		FROM User u
		LEFT JOIN Company c ON u.companyId = c.id
	`
	args := []interface{}{}

	// Non-admin users can only see users from their company
	if userRole != models.RoleAdmin && userRole != models.RoleFDA {
		if companyExists {
			query += " WHERE u.companyId = ?"
			args = append(args, userCompanyID)
		}
	}

	query += " ORDER BY u.createdAt DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		var user models.User
		var fullName, designation, department, username, image, phone, companyID sql.NullString
		var lastLoginAt sql.NullTime

		// Company fields
		var cID, cName, cEmail, cPhone, cAddress, cCountry sql.NullString
		var cLogo, cStatus, cSubscriptionID sql.NullString
		var cCreatedAt, cUpdatedAt sql.NullTime

		err := rows.Scan(
			&user.ID, &user.Email, &user.Role, &fullName, &designation,
			&department, &username, &image, &phone, &user.IsActive,
			&companyID, &lastLoginAt, &user.CreatedAt, &user.UpdatedAt,
			&cID, &cName, &cEmail, &cPhone, &cAddress, &cCountry,
			&cLogo, &cStatus, &cSubscriptionID, &cCreatedAt, &cUpdatedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning user"})
			return
		}

		// Map nullable user fields
		if fullName.Valid {
			user.FullName = &fullName.String
		}
		if designation.Valid {
			user.Designation = &designation.String
		}
		if department.Valid {
			user.Department = &department.String
		}
		if username.Valid {
			user.Username = &username.String
		}
		if image.Valid {
			user.Image = &image.String
		}
		if phone.Valid {
			user.Phone = &phone.String
		}
		if companyID.Valid {
			user.CompanyID = &companyID.String
		}
		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}

		// Map company if exists
		if cID.Valid {
			company := &models.Company{
				ID:     cID.String,
				Name:   cName.String,
				Email:  cEmail.String,
				Status: cStatus.String,
			}
			if cPhone.Valid {
				company.Phone = &cPhone.String
			}
			if cAddress.Valid {
				company.Address = &cAddress.String
			}
			if cCountry.Valid {
				company.Country = &cCountry.String
			}
			if cLogo.Valid {
				company.Logo = &cLogo.String
			}
			if cSubscriptionID.Valid {
				company.SubscriptionID = &cSubscriptionID.String
			}
			if cCreatedAt.Valid {
				company.CreatedAt = cCreatedAt.Time
			}
			if cUpdatedAt.Valid {
				company.UpdatedAt = cUpdatedAt.Time
			}
			user.Company = company
		}

		users = append(users, user)
	}

	c.JSON(http.StatusOK, users)
}

// GetUserByID retrieves a specific user
func (h *UserHandler) GetUserByID(c *gin.Context) {
	id := c.Param("id")

	user, err := h.getUserByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// GetUserByEmail retrieves a user by email
func (h *UserHandler) GetUserByEmail(c *gin.Context) {
	email := c.Param("email")

	user, err := h.getUserByEmail(email)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// GetUsersByCompanyID retrieves all users for a company
func (h *UserHandler) GetUsersByCompanyID(c *gin.Context) {
	companyID := c.Param("companyId")

	query := `
		SELECT id, email, role, fullName, designation, department, username, image, 
		       phone, isActive, companyId, lastLoginAt, createdAt, updatedAt 
		FROM User WHERE companyId = ? ORDER BY createdAt DESC
	`

	rows, err := h.db.Query(query, companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
		return
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		var user models.User
		var fullName, designation, department, username, image, phone, cID sql.NullString
		var lastLoginAt sql.NullTime

		err := rows.Scan(
			&user.ID, &user.Email, &user.Role, &fullName, &designation,
			&department, &username, &image, &phone, &user.IsActive,
			&cID, &lastLoginAt, &user.CreatedAt, &user.UpdatedAt,
		)
		if err != nil {
			continue
		}

		// Map nullable fields
		if fullName.Valid {
			user.FullName = &fullName.String
		}
		if designation.Valid {
			user.Designation = &designation.String
		}
		if department.Valid {
			user.Department = &department.String
		}
		if username.Valid {
			user.Username = &username.String
		}
		if image.Valid {
			user.Image = &image.String
		}
		if phone.Valid {
			user.Phone = &phone.String
		}
		if cID.Valid {
			user.CompanyID = &cID.String
		}
		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}

		users = append(users, user)
	}

	c.JSON(http.StatusOK, users)
}

// CreateUser creates a new user
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("CreateUser: Error binding JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("CreateUser: Creating user with email: %s, role: %s", req.Email, req.Role)

	// Validate role
	validRoles := []string{models.RoleAdmin, models.RoleFDA, models.RoleGatekeeper, models.RoleUser}
	isValidRole := false
	for _, role := range validRoles {
		if req.Role == role {
			isValidRole = true
			break
		}
	}
	if !isValidRole {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role. Must be one of: admin, fda, gatekeeper, user"})
		return
	}

	// Check if company exists if companyID is provided
	if req.CompanyID != nil {
		var companyExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM Company WHERE id = ?)", *req.CompanyID).Scan(&companyExists)
		if err != nil || !companyExists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
			return
		}
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("CreateUser: Error hashing password: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error hashing password"})
		return
	}

	// Generate ID and timestamps
	id := uuid.New().String()
	now := time.Now()

	// Insert user with new schema
	query := `
		INSERT INTO User (
			id, email, role, fullName, username, password, phone, designation, 
			department, isActive, companyId, createdAt, updatedAt
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
	`

	_, err = h.db.Exec(query,
		id, req.Email, req.Role, req.FullName, req.Username, string(hashedPassword),
		req.Phone, req.Designation, req.Department, req.CompanyID, now, now,
	)
	if err != nil {
		log.Printf("CreateUser: Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating user", "details": err.Error()})
		return
	}

	log.Printf("CreateUser: User created successfully with ID: %s", id)

	// Fetch and return created user
	user, err := h.getUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User created but failed to fetch"})
		return
	}

	c.JSON(http.StatusCreated, user)
}

// UpdateUser updates an existing user
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	log.Printf("UpdateUser: Received request for user ID: %s", id)

	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("UpdateUser: Error binding JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()

	// Build dynamic update query
	query := "UPDATE User SET updatedAt = ?"
	args := []interface{}{now}

	if req.FullName != nil {
		query += ", fullName = ?"
		args = append(args, *req.FullName)
	}
	if req.Username != nil {
		query += ", username = ?"
		args = append(args, *req.Username)
	}
	if req.Email != nil {
		query += ", email = ?"
		args = append(args, *req.Email)
	}
	if req.Phone != nil {
		query += ", phone = ?"
		args = append(args, *req.Phone)
	}
	if req.Department != nil {
		query += ", department = ?"
		args = append(args, *req.Department)
	}
	if req.Designation != nil {
		query += ", designation = ?"
		args = append(args, *req.Designation)
	}
	if req.Role != nil {
		query += ", role = ?"
		args = append(args, *req.Role)
	}
	if req.CompanyID != nil {
		query += ", companyId = ?"
		args = append(args, *req.CompanyID)
	}
	if req.IsActive != nil {
		query += ", isActive = ?"
		args = append(args, *req.IsActive)
	}
	if req.Password != nil {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error hashing password"})
			return
		}
		query += ", password = ?"
		args = append(args, string(hashedPassword))
	}

	query += " WHERE id = ?"
	args = append(args, id)

	result, err := h.db.Exec(query, args...)
	if err != nil {
		log.Printf("UpdateUser: Error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Fetch and return updated user
	user, err := h.getUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User updated but failed to fetch"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// DeleteUser deletes a user
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")

	query := "DELETE FROM User WHERE id = ?"
	result, err := h.db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting user", "details": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

// ActivateUser activates a user account
func (h *UserHandler) ActivateUser(c *gin.Context) {
	id := c.Param("id")

	query := "UPDATE User SET isActive = 1, updatedAt = ? WHERE id = ?"
	result, err := h.db.Exec(query, time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error activating user"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User activated successfully"})
}

// DeactivateUser deactivates a user account
func (h *UserHandler) DeactivateUser(c *gin.Context) {
	id := c.Param("id")

	query := "UPDATE User SET isActive = 0, updatedAt = ? WHERE id = ?"
	result, err := h.db.Exec(query, time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deactivating user"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deactivated successfully"})
}

// Helper function to get user by ID
func (h *UserHandler) getUserByID(id string) (*models.User, error) {
	var user models.User
	var fullName, designation, department, username, image, phone, companyID sql.NullString
	var lastLoginAt sql.NullTime

	query := `
		SELECT id, email, role, fullName, designation, department, username, image, 
		       phone, isActive, companyId, lastLoginAt, createdAt, updatedAt 
		FROM User WHERE id = ?
	`

	err := h.db.QueryRow(query, id).Scan(
		&user.ID, &user.Email, &user.Role, &fullName, &designation,
		&department, &username, &image, &phone, &user.IsActive,
		&companyID, &lastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Map nullable fields
	if fullName.Valid {
		user.FullName = &fullName.String
	}
	if designation.Valid {
		user.Designation = &designation.String
	}
	if department.Valid {
		user.Department = &department.String
	}
	if username.Valid {
		user.Username = &username.String
	}
	if image.Valid {
		user.Image = &image.String
	}
	if phone.Valid {
		user.Phone = &phone.String
	}
	if companyID.Valid {
		user.CompanyID = &companyID.String

		// Fetch company if companyID exists
		if companyID.String != "" {
			var company models.Company
			companyQuery := `SELECT id, name, createdAt, updatedAt FROM Company WHERE id = ?`
			err := h.db.QueryRow(companyQuery, companyID.String).Scan(
				&company.ID, &company.Name, &company.CreatedAt, &company.UpdatedAt,
			)
			if err == nil {
				user.Company = &company
			}
		}
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return &user, nil
}

// Helper function to get user by email
func (h *UserHandler) getUserByEmail(email string) (*models.User, error) {
	var user models.User
	var fullName, designation, department, username, image, phone, companyID sql.NullString
	var lastLoginAt sql.NullTime

	query := `
		SELECT id, email, role, fullName, designation, department, username, image, 
		       phone, isActive, companyId, lastLoginAt, createdAt, updatedAt 
		FROM User WHERE email = ?
	`

	err := h.db.QueryRow(query, email).Scan(
		&user.ID, &user.Email, &user.Role, &fullName, &designation,
		&department, &username, &image, &phone, &user.IsActive,
		&companyID, &lastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Map nullable fields
	if fullName.Valid {
		user.FullName = &fullName.String
	}
	if designation.Valid {
		user.Designation = &designation.String
	}
	if department.Valid {
		user.Department = &department.String
	}
	if username.Valid {
		user.Username = &username.String
	}
	if image.Valid {
		user.Image = &image.String
	}
	if phone.Valid {
		user.Phone = &phone.String
	}
	if companyID.Valid {
		user.CompanyID = &companyID.String

		// Fetch company if companyID exists
		if companyID.String != "" {
			var company models.Company
			companyQuery := `SELECT id, name, createdAt, updatedAt FROM Company WHERE id = ?`
			err := h.db.QueryRow(companyQuery, companyID.String).Scan(
				&company.ID, &company.Name, &company.CreatedAt, &company.UpdatedAt,
			)
			if err == nil {
				user.Company = &company
			}
		}
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return &user, nil
}
