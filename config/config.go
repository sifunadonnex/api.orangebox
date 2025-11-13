package config

import "os"

// GetPort returns the port to run the server on
func GetPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	return port
}

// GetBearerToken returns the authentication bearer token
func GetBearerToken() string {
	token := os.Getenv("BEARER_TOKEN")
	if token == "" {
		token = "a6bca59a8855b4"
	}
	return token
}

// GetJWTSecret returns the JWT secret key
func GetJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "your-secret-key-change-in-production-2024-fdm-system"
	}
	return secret
}
