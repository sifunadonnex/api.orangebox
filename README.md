# FDM API - Go Version

Flight Data Monitoring (FDM) Backend API built with Go, Gin web framework, and SQLite database.

## Features

- User authentication with bcrypt password hashing
- Aircraft management
- CSV file upload and management
- Event logging and management
- Exceedance tracking and reporting
- RESTful API endpoints
- CORS support for frontend integration

## Technologies Used

- **Go 1.21+** - Programming language
- **Gin** - Web framework
- **SQLite** - Database (using existing Prisma database)
- **bcrypt** - Password hashing
- **UUID** - Unique identifier generation

## API Endpoints

### Authentication
- `POST /login` - User login

### Users
- `GET /users` - Get all users
- `POST /users` - Create new user
- `PUT /users/:id` - Update user
- `GET /users/:id` - Get user by ID
- `GET /users/gate/:id` - Get users by gate ID
- `GET /user/:id` - Get user by email
- `DELETE /users/:id` - Delete user

### Aircraft
- `GET /aircrafts` - Get all aircraft
- `GET /aircrafts/:id` - Get aircraft by user ID
- `POST /aircrafts` - Create new aircraft
- `PUT /aircrafts/:id` - Update aircraft
- `DELETE /aircrafts/:id` - Delete aircraft

### CSV Files
- `POST /csv` - Upload CSV file
- `GET /csv` - Get all CSV files
- `GET /csv/:id` - Download CSV file
- `GET /flight/:id` - Get CSV by ID
- `DELETE /csv/:id` - Delete CSV

### Events
- `POST /events` - Create new event
- `GET /events` - Get all events
- `GET /events/:id` - Get event by ID
- `PUT /events/:id` - Update event
- `DELETE /events/:id` - Delete event

### Exceedances
- `GET /exceedances` - Get all exceedances
- `GET /exceedances/:id` - Get exceedance by ID
- `GET /exceedances/flight/:id` - Get exceedances by flight ID
- `POST /exceedances` - Create exceedances
- `PUT /exceedances/:id` - Update exceedance
- `DELETE /exceedances/:id` - Delete exceedance

## Installation and Setup

1. **Install Go** (version 1.21 or higher)

2. **Initialize Go modules and install dependencies:**
   ```bash
   go mod tidy
   ```

3. **Ensure the Prisma database exists:**
   The application uses the existing SQLite database at `prisma/dev.db`

4. **Run the application:**
   ```bash
   go run main.go
   ```

5. **The server will start on:** `http://localhost:8000`

## Environment Variables

- `PORT` - Server port (default: 8000)
- `BEARER_TOKEN` - Authentication bearer token (default: a6bca59a8855b4)

## Project Structure

```
├── main.go              # Application entry point
├── go.mod              # Go module file
├── config/             # Configuration files
│   └── config.go       # Environment configuration
├── database/           # Database connection
│   └── database.go     # Database initialization
├── handlers/           # HTTP request handlers
│   ├── user.go         # User-related endpoints
│   ├── aircraft.go     # Aircraft-related endpoints
│   ├── csv.go          # CSV file-related endpoints
│   ├── event.go        # Event-related endpoints
│   └── exceedance.go   # Exceedance-related endpoints
├── middleware/         # HTTP middleware
│   └── auth.go         # Authentication middleware
├── models/             # Data models and DTOs
│   └── models.go       # All data structures
├── prisma/             # Database files (from previous Node.js setup)
│   ├── dev.db          # SQLite database
│   └── schema.prisma   # Database schema
├── csvs/               # CSV file uploads directory
└── public/             # Static files
    └── index.html      # Default HTML page
```

## Authentication

The API uses a simple bearer token authentication. Include the token in the `Authorization` header:

```
Authorization: a6bca59a8855b4
```

## File Uploads

CSV files are uploaded to the `/csvs` directory and served statically. The upload endpoint accepts multipart/form-data with the file and metadata.

## Database Schema

The application uses the existing Prisma database schema with the following main entities:
- Users
- Aircraft
- CSV files
- Events
- Exceedances

All relationships and constraints are maintained as defined in the original Prisma schema.
"# api.orangebox" 
