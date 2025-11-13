# FDM Backend API Reference

Base URL: `http://localhost:8000`

## Table of Contents
- [Authentication](#authentication)
- [Companies](#companies)
- [Subscriptions](#subscriptions)
- [Users](#users)
- [Aircraft](#aircraft)
- [Flights/CSV](#flightscsv)
- [Events](#events)
- [Exceedances](#exceedances)
- [Notifications](#notifications)

---

## Authentication

### Login
```http
POST /login
```

**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "password123"
}
```

**Response:** (200 OK)
```json
{
  "user": {
    "id": "123",
    "email": "user@example.com",
    "role": "admin",
    "fullName": "John Doe",
    "isActive": true,
    "companyId": "456",
    "company": {
      "id": "456",
      "name": "Demo Aviation",
      "status": "active"
    }
  },
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "message": "Login successful"
}
```

**Error Responses:**
- `401 Unauthorized`: Invalid credentials
- `403 Forbidden`: Account is deactivated or company suspended/expired

**Use the token in all subsequent requests:**
```http
Authorization: Bearer {token}
```

---

## Companies

All company endpoints require **Admin role**.

### Create Company
```http
POST /api/companies
Authorization: Bearer {token}
```

**Request Body:**
```json
{
  "name": "Aviation Company",
  "email": "contact@aviation.com",
  "phone": "+1-555-0100",
  "address": "123 Airport Rd",
  "country": "United States",
  "logo": "https://example.com/logo.png",
  "subscriptionId": "sub-123"
}
```

**Response:** (201 Created)
```json
{
  "id": "comp-123",
  "name": "Aviation Company",
  "email": "contact@aviation.com",
  "phone": "+1-555-0100",
  "address": "123 Airport Rd",
  "country": "United States",
  "logo": "https://example.com/logo.png",
  "status": "active",
  "subscriptionId": "sub-123",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### List Companies
```http
GET /api/companies
Authorization: Bearer {token}
```

**Response:** (200 OK)
```json
[
  {
    "id": "comp-123",
    "name": "Aviation Company",
    "email": "contact@aviation.com",
    "status": "active",
    "userCount": 15,
    "aircraftCount": 5,
    "subscription": {
      "id": "sub-123",
      "planName": "Professional",
      "maxUsers": 50,
      "maxAircraft": 20,
      "endDate": "2025-01-01T00:00:00Z"
    }
  }
]
```

### Get Company by ID
```http
GET /api/companies/{id}
Authorization: Bearer {token}
```

**Response:** (200 OK)
```json
{
  "id": "comp-123",
  "name": "Aviation Company",
  "email": "contact@aviation.com",
  "phone": "+1-555-0100",
  "address": "123 Airport Rd",
  "country": "United States",
  "logo": "https://example.com/logo.png",
  "status": "active",
  "subscriptionId": "sub-123",
  "userCount": 15,
  "aircraftCount": 5,
  "subscription": { /* subscription details */ },
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### Update Company
```http
PUT /api/companies/{id}
Authorization: Bearer {token}
```

**Request Body:** (All fields optional)
```json
{
  "name": "New Company Name",
  "email": "newemail@aviation.com",
  "phone": "+1-555-0200",
  "address": "456 New Address",
  "country": "Canada",
  "logo": "https://example.com/newlogo.png",
  "subscriptionId": "sub-456"
}
```

### Suspend Company
```http
PUT /api/companies/{id}/suspend
Authorization: Bearer {token}
```

**Response:** (200 OK)
```json
{
  "message": "Company suspended successfully"
}
```

**Effect:** All company users will be unable to login.

### Activate Company
```http
PUT /api/companies/{id}/activate
Authorization: Bearer {token}
```

**Response:** (200 OK)
```json
{
  "message": "Company activated successfully"
}
```

### Delete Company
```http
DELETE /api/companies/{id}
Authorization: Bearer {token}
```

**Response:** (200 OK)
```json
{
  "message": "Company deleted successfully"
}
```

**Note:** Cannot delete company with existing users. Must delete users first.

---

## Subscriptions

All subscription endpoints require **Admin role**.

### Create Subscription
```http
POST /api/subscriptions
Authorization: Bearer {token}
```

**Request Body:**
```json
{
  "planName": "Professional Plan",
  "planType": "professional",
  "maxUsers": 50,
  "maxAircraft": 20,
  "maxFlightsPerMonth": 1000,
  "maxStorageGB": 500,
  "price": 999.99,
  "currency": "USD",
  "startDate": "2024-01-01T00:00:00Z",
  "endDate": "2025-01-01T00:00:00Z",
  "isActive": true,
  "autoRenew": true
}
```

**Response:** (201 Created)
```json
{
  "id": "sub-123",
  "planName": "Professional Plan",
  "planType": "professional",
  "maxUsers": 50,
  "maxAircraft": 20,
  "maxFlightsPerMonth": 1000,
  "maxStorageGB": 500,
  "price": 999.99,
  "currency": "USD",
  "startDate": "2024-01-01T00:00:00Z",
  "endDate": "2025-01-01T00:00:00Z",
  "isActive": true,
  "autoRenew": true,
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### List Subscriptions
```http
GET /api/subscriptions
Authorization: Bearer {token}
```

**Query Parameters:**
- `isActive` (optional): Filter by active status (true/false)

**Response:** (200 OK)
```json
[
  {
    "id": "sub-123",
    "planName": "Professional Plan",
    "maxUsers": 50,
    "maxAircraft": 20,
    "price": 999.99,
    "endDate": "2025-01-01T00:00:00Z",
    "isActive": true,
    "companyCount": 10
  }
]
```

### Get Subscription by ID
```http
GET /api/subscriptions/{id}
Authorization: Bearer {token}
```

### Update Subscription
```http
PUT /api/subscriptions/{id}
Authorization: Bearer {token}
```

**Request Body:** (All fields optional)
```json
{
  "planName": "Updated Plan Name",
  "maxUsers": 100,
  "price": 1499.99,
  "endDate": "2026-01-01T00:00:00Z"
}
```

### Get Subscription Status (Usage)
```http
GET /api/subscriptions/{id}/status
Authorization: Bearer {token}
```

**Response:** (200 OK)
```json
{
  "subscriptionId": "sub-123",
  "planName": "Professional Plan",
  "currentUsers": 35,
  "maxUsers": 50,
  "usersLimitReached": false,
  "currentAircraft": 12,
  "maxAircraft": 20,
  "aircraftLimitReached": false,
  "currentFlights": 450,
  "maxFlights": 1000,
  "flightsLimitReached": false,
  "currentStorage": 256.5,
  "maxStorage": 500,
  "storageLimitReached": false,
  "daysUntilExpiry": 45,
  "isExpiringSoon": false
}
```

### Check Expired Subscriptions
```http
POST /api/subscriptions/check-expired
Authorization: Bearer {token}
```

**Response:** (200 OK)
```json
{
  "message": "Checked 50 subscriptions",
  "expiredCount": 3,
  "alertsSent": 5,
  "companiesSuspended": 3
}
```

**Actions Performed:**
- Sends alerts for subscriptions expiring in 7 days
- Suspends companies with expired subscriptions
- Updates company status to "expired"

---

## Users

### List Users
```http
GET /api/users
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

**Response:** (200 OK)
```json
[
  {
    "id": "user-123",
    "email": "user@example.com",
    "role": "gatekeeper",
    "fullName": "John Doe",
    "username": "johndoe",
    "phone": "+1-555-0123",
    "designation": "Flight Data Analyst",
    "department": "Operations",
    "isActive": true,
    "companyId": "comp-123",
    "lastLoginAt": "2024-01-15T10:30:00Z",
    "createdAt": "2024-01-01T00:00:00Z"
  }
]
```

**Note:** Non-admin users only see users from their own company.

### Create User
```http
POST /api/users
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

**Request Body:**
```json
{
  "email": "newuser@example.com",
  "password": "SecurePass123",
  "role": "user",
  "fullName": "Jane Smith",
  "username": "janesmith",
  "phone": "+1-555-0456",
  "designation": "Pilot",
  "department": "Flight Operations",
  "companyId": "comp-123"
}
```

**Response:** (201 Created)
```json
{
  "id": "user-456",
  "email": "newuser@example.com",
  "role": "user",
  "fullName": "Jane Smith",
  "username": "janesmith",
  "phone": "+1-555-0456",
  "designation": "Pilot",
  "department": "Flight Operations",
  "isActive": true,
  "companyId": "comp-123",
  "createdAt": "2024-01-20T00:00:00Z"
}
```

### Get User by ID
```http
GET /api/users/{id}
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user (can view own profile)

### Update User
```http
PUT /api/users/{id}
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

**Request Body:** (All fields optional)
```json
{
  "fullName": "Updated Name",
  "username": "newusername",
  "email": "newemail@example.com",
  "phone": "+1-555-9999",
  "designation": "Senior Pilot",
  "department": "Training",
  "role": "gatekeeper",
  "password": "NewPassword123",
  "isActive": false
}
```

### Delete User
```http
DELETE /api/users/{id}
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA

### Activate User
```http
PUT /api/users/{id}/activate
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA

**Response:** (200 OK)
```json
{
  "message": "User activated successfully"
}
```

### Deactivate User
```http
PUT /api/users/{id}/deactivate
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA

**Response:** (200 OK)
```json
{
  "message": "User deactivated successfully"
}
```

### Get Users by Company
```http
GET /api/users/company/{companyId}
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

---

## Aircraft

### List Aircraft
```http
GET /api/aircrafts
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

**Response:** (200 OK)
```json
[
  {
    "id": "aircraft-123",
    "airline": "Demo Aviation",
    "aircraftMake": "Boeing",
    "modelNumber": "737-800",
    "serialNumber": "SN12345",
    "registration": "N123AB",
    "companyId": "comp-123",
    "parameters": "{...}",
    "createdAt": "2024-01-01T00:00:00Z"
  }
]
```

**Note:** Users see only their company's aircraft (except admin/fda).

### Create Aircraft
```http
POST /api/aircrafts
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

**Request Body:**
```json
{
  "airline": "Demo Aviation",
  "aircraftMake": "Airbus",
  "modelNumber": "A320-200",
  "serialNumber": "SN67890",
  "registration": "N456CD",
  "companyId": "comp-123",
  "parameters": "{\"engineType\":\"CFM56\",\"maxWeight\":78000}"
}
```

### Get Aircraft by User ID
```http
GET /api/aircrafts/{id}
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

**Note:** This endpoint name is legacy; it actually returns aircraft by aircraft ID.

### Update Aircraft
```http
PUT /api/aircrafts/{id}
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

### Delete Aircraft
```http
DELETE /api/aircrafts/{id}
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA

---

## Flights/CSV

### Upload CSV (Flight Data)
```http
POST /api/csv
Authorization: Bearer {token}
Content-Type: multipart/form-data
```

**Required Role:** Gatekeeper or above

**Form Data:**
- `file`: CSV file (max 8MB)
- `aircraftId`: Aircraft ID
- `pilot`: Pilot name (optional)
- `departure`: Departure airport (optional)
- `destination`: Destination airport (optional)
- `flightHours`: Flight duration (optional)

**Response:** (201 Created)
```json
{
  "id": "csv-123",
  "name": "flight_2024_01_20.csv",
  "file": "/csvs/flight_2024_01_20.csv",
  "status": "uploaded",
  "aircraftId": "aircraft-123",
  "pilot": "John Doe",
  "departure": "JFK",
  "destination": "LAX",
  "flightHours": "5.5",
  "createdAt": "2024-01-20T00:00:00Z"
}
```

### List CSVs
```http
GET /api/csv
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

### Download CSV
```http
GET /api/csv/{id}
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

**Response:** CSV file download

### Get Flight by ID
```http
GET /api/flight/{id}
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

### Delete CSV
```http
DELETE /api/csv/{id}
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA

---

## Events

### List Events
```http
GET /api/events
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

**Response:** (200 OK)
```json
[
  {
    "id": "event-123",
    "eventName": "Hard Landing",
    "severity": "high",
    "flightId": "flight-123",
    "aircraftId": "aircraft-123",
    "validationStatus": "pending",
    "createdBy": "user-456",
    "createdAt": "2024-01-20T00:00:00Z"
  }
]
```

### Create Event
```http
POST /api/events
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

**Request Body:**
```json
{
  "eventName": "Hard Landing",
  "severity": "high",
  "description": "Landing exceeded normal G-force limits",
  "flightId": "flight-123",
  "aircraftId": "aircraft-123",
  "detectionPeriod": "landing",
  "triggerType": "automatic"
}
```

### Get Event by ID
```http
GET /api/events/{id}
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

### Update Event
```http
PUT /api/events/{id}
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA (for validation)

**Request Body:**
```json
{
  "validationStatus": "validated",
  "severity": "medium",
  "notes": "Reviewed and classified as acceptable."
}
```

### Delete Event
```http
DELETE /api/events/{id}
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA

---

## Exceedances

### List Exceedances
```http
GET /api/exceedances
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

### Get Exceedance by ID
```http
GET /api/exceedances/{id}
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

### Get Exceedances by Flight
```http
GET /api/exceedances/flight/{flightId}
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

### Create Exceedances (Bulk)
```http
POST /api/exceedances
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

**Request Body:**
```json
{
  "exceedances": [
    {
      "parameterName": "Altitude",
      "exceedanceValue": 42000,
      "threshold": 41000,
      "flightId": "flight-123",
      "aircraftId": "aircraft-123",
      "timestamp": "2024-01-20T14:30:00Z"
    }
  ]
}
```

### Update Exceedance
```http
PUT /api/exceedances/{id}
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA

### Delete Exceedance
```http
DELETE /api/exceedances/{id}
Authorization: Bearer {token}
```

**Required Role:** Admin or FDA

---

## Notifications

### Create Notifications
```http
POST /api/notifications
Authorization: Bearer {token}
```

**Required Role:** Gatekeeper or above

**Request Body:**
```json
{
  "userId": "user-123",
  "title": "New Event Detected",
  "message": "A hard landing event was detected on flight FL123",
  "type": "event",
  "relatedId": "event-456"
}
```

### Get User Notifications
```http
GET /api/notifications/user/{userId}
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user (own notifications only)

**Response:** (200 OK)
```json
[
  {
    "id": "notif-123",
    "userId": "user-123",
    "title": "New Event Detected",
    "message": "A hard landing event was detected on flight FL123",
    "type": "event",
    "relatedId": "event-456",
    "isRead": false,
    "createdAt": "2024-01-20T15:00:00Z"
  }
]
```

### Mark Notification as Read
```http
PUT /api/notifications/{id}/read
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

### Mark All Notifications as Read
```http
PUT /api/notifications/user/{userId}/mark-all-read
Authorization: Bearer {token}
```

**Required Role:** Any authenticated user

**Response:** (200 OK)
```json
{
  "message": "All notifications marked as read",
  "count": 12
}
```

---

## Role Permissions Summary

| Endpoint Category | Admin | FDA | Gatekeeper | User |
|------------------|-------|-----|------------|------|
| Companies | ✅ Full | ❌ | ❌ | ❌ |
| Subscriptions | ✅ Full | ❌ | ❌ | ❌ |
| Users - View | ✅ All | ✅ All | ✅ Own Co. | ✅ Own |
| Users - Create/Update | ✅ | ✅ | ✅ | ❌ |
| Users - Delete | ✅ | ✅ | ❌ | ❌ |
| Aircraft - View | ✅ All | ✅ All | ✅ Own Co. | ✅ Own Co. |
| Aircraft - Create/Update | ✅ | ✅ | ✅ | ❌ |
| Aircraft - Delete | ✅ | ✅ | ❌ | ❌ |
| Flights - View | ✅ | ✅ | ✅ | ✅ |
| Flights - Upload | ✅ | ✅ | ✅ | ❌ |
| Flights - Delete | ✅ | ✅ | ❌ | ❌ |
| Events - View | ✅ | ✅ | ✅ | ✅ |
| Events - Create | ✅ | ✅ | ✅ | ❌ |
| Events - Validate | ✅ | ✅ | ❌ | ❌ |
| Events - Delete | ✅ | ✅ | ❌ | ❌ |
| Exceedances - View | ✅ | ✅ | ✅ | ✅ |
| Exceedances - Create | ✅ | ✅ | ✅ | ❌ |
| Exceedances - Update/Delete | ✅ | ✅ | ❌ | ❌ |
| Notifications - View | ✅ Own | ✅ Own | ✅ Own | ✅ Own |
| Notifications - Create | ✅ | ✅ | ✅ | ❌ |

---

## Error Responses

### 400 Bad Request
```json
{
  "error": "Validation error message"
}
```

### 401 Unauthorized
```json
{
  "error": "Invalid or expired token"
}
```

### 403 Forbidden
```json
{
  "error": "Insufficient permissions",
  "required": "admin"
}
```

or

```json
{
  "error": "Company account is suspended",
  "message": "Please contact support to reactivate your account"
}
```

### 404 Not Found
```json
{
  "error": "Resource not found"
}
```

### 500 Internal Server Error
```json
{
  "error": "Internal server error",
  "details": "Error message"
}
```

---

## Rate Limiting
Currently not implemented. Consider adding rate limiting in production.

## CORS
Configured for:
- `http://localhost:3000`
- `https://www.orangebox.co.ke`
- `http://www.orangebox.co.ke`

## File Uploads
- Max size: 8 MB
- Supported formats: CSV
- Storage: `./csvs/` directory

---

## Testing Tips

### Using cURL
```bash
# Login
TOKEN=$(curl -s -X POST http://localhost:8000/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@fdm.com","password":"Admin@123"}' \
  | jq -r '.token')

# Use token
curl http://localhost:8000/api/companies \
  -H "Authorization: Bearer $TOKEN"
```

### Using Postman
1. Create environment variable `token`
2. In Tests tab of login request:
   ```javascript
   pm.environment.set("token", pm.response.json().token);
   ```
3. Use `{{token}}` in Authorization header

### Using JavaScript/Fetch
```javascript
// Store token after login
const { token } = await response.json();
localStorage.setItem('authToken', token);

// Use in requests
fetch('http://localhost:8000/api/users', {
  headers: {
    'Authorization': `Bearer ${localStorage.getItem('authToken')}`
  }
});
```

---

**Server:** `http://localhost:8000`  
**Status:** ✅ Running  
**Version:** 1.0.0  
**Last Updated:** November 12, 2024
