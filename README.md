# FDM API - Flight Data Monitoring Backend

Production-ready Flight Data Monitoring (FDM) Backend API built with Go, Gin framework, and SQLite/PostgreSQL.

## Features

- 🔐 **JWT Authentication** with session management
- 👤 **Single Device Login** - Prevents credential sharing
- 🏢 **Multi-tenant** - Company-based data isolation
- ✈️ **Aircraft Management** - Fleet tracking
- 📊 **Flight Data Analysis** - CSV upload and processing
- ⚠️ **Exceedance Tracking** - Event monitoring and alerts
- 🔔 **Notifications** - Real-time alert system
- 📦 **Subscription Management** - Plan-based access control

## Tech Stack

- **Go 1.21+** - Programming language
- **Gin** - High-performance web framework
- **SQLite/PostgreSQL** - Database
- **JWT** - Token-based authentication
- **bcrypt** - Password hashing

## Quick Start

### Prerequisites
- Go 1.21 or higher
- Git

### Installation

```bash
# Clone the repository
git clone https://github.com/sifunadonnex/api.orangebox.git
cd api.orangebox

# Copy environment file
cp .env.example .env

# Edit .env with your configuration
# - Set ACCESS_TOKEN_SECRET to a secure random string
# - Configure DATABASE_URL for your database

# Install dependencies
go mod tidy

# Run the application
go run .
```

The server starts on `http://localhost:8000`

### Production Build

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o api-server .

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o api-server.exe .

# Run in production mode
GIN_MODE=release ./api-server
```

## API Endpoints

### Authentication
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/login` | User login (returns JWT token) |
| POST | `/api/logout` | Logout current session |
| POST | `/api/logout-all` | Logout from all devices |
| GET | `/api/sessions` | View active sessions |

### Users
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/users` | List all users |
| POST | `/api/users` | Create user |
| GET | `/api/users/:id` | Get user by ID |
| PUT | `/api/users/:id` | Update user |
| DELETE | `/api/users/:id` | Delete user |

### Companies
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/companies` | List companies |
| POST | `/api/companies` | Create company |
| PUT | `/api/companies/:id` | Update company |
| DELETE | `/api/companies/:id` | Delete company |

### Aircraft
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/aircrafts` | List aircraft |
| POST | `/api/aircrafts` | Create aircraft |
| GET | `/api/aircrafts/:id` | Get aircraft |
| PUT | `/api/aircrafts/:id` | Update aircraft |
| DELETE | `/api/aircrafts/:id` | Delete aircraft |

### Flight Data (CSV)
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/csv` | Upload flight data |
| GET | `/api/csv` | List all flights |
| GET | `/api/csv/:id` | Download CSV |
| DELETE | `/api/csv/:id` | Delete flight |

### Events & Exceedances
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/events` | List events |
| POST | `/api/events` | Create event |
| GET | `/api/exceedances` | List exceedances |
| POST | `/api/exceedances` | Create exceedances |

### Notifications
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/notifications/user/:userId` | Get user notifications |
| PUT | `/api/notifications/:id/read` | Mark as read |

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ACCESS_TOKEN_SECRET` | JWT signing secret | Required |
| `DATABASE_URL` | Database connection string | `file:./prisma/dev.db` |
| `PORT` | Server port | `8000` |
| `GIN_MODE` | Gin mode (debug/release) | `debug` |

## Project Structure

```
├── main.go                 # Application entry point
├── config/                 # Configuration
├── database/               # Database connection & migrations
├── handlers/               # HTTP request handlers
├── middleware/             # Authentication & authorization
├── models/                 # Data models
├── utils/                  # Utility functions (JWT, etc.)
├── prisma/                 # Database schema
└── csvs/                   # Uploaded flight data files
```

## Security Features

### Single Device Login
- Only one active session per user account
- Logging in from a new device automatically invalidates previous sessions
- Users receive clear feedback when kicked out

### Role-Based Access Control
- **Admin** - Full system access
- **FDA** - Flight Data Analyst access
- **Gatekeeper** - Company-level management
- **User** - Basic read access

### Company Data Isolation
- Users can only access data within their company
- Cross-company data access is prevented at the middleware level

## License

MIT License
