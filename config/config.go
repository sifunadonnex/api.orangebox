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
