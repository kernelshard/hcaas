# 🚑 HealthCheck as a Service (HCaaS)

![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8)  
![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)  
![Build Status](https://img.shields.io/badge/build-passing-brightgreen)

A **Golang-based microservice** for registering and monitoring the health of HTTP URLs. Built with **idiomatic Go** and **clean architecture**, it’s designed for production-ready reliability and maintainability.

---

## ✨ Features

- Register and track the health of HTTP URLs
- Manually update URL status (automated cron checks planned)
- Clean architecture (Handler → Service → Storage)
- PostgreSQL storage via `pgxpool`
- Idiomatic, production-grade Go codebase

---

## 📁 Project Structure

```
hcaas/
├── cmd/
│   └── server/
│       └── main.go         # Application entry point
├── handler/                # HTTP handlers (Chi-based)
├── model/                  # URL struct definitions
├── service/                # Business logic
├── storage/                # PostgreSQL storage layer
├── .env                    # Environment variable config
├── .env.example            # Example environment config
├── go.mod                  # Go module dependencies
├── go.sum                  # Dependency checksums
├── README.md               # Project documentation
```

---

## 🛠️ Tech Stack

- **Language**: [Go 1.22+](https://golang.org/)
- **HTTP Router**: [Chi](https://github.com/go-chi/chi)
- **Database**: [PostgreSQL](https://www.postgresql.org/) (via [pgxpool](https://github.com/jackc/pgx))
- **Architecture**: Clean layered architecture (Handler → Service → Storage)

---

## 🚀 Getting Started

### Prerequisites
- **Go**: 1.22 or higher
- **PostgreSQL**: A running instance (local or remote)
- **Git**: For cloning the repository

### 1. Clone the Repository

```bash
git clone https://github.com/kernelshard/hcaas
cd hcaas
```

### 2. Configure Environment

Copy the example environment file:

```bash
cp .env.example .env
```

Update `.env` with your PostgreSQL configuration:

```bash
DATABASE_URL=postgres://username:password@localhost:5432/yourdb?sslmode=disable
```

Replace `username`, `password`, and `yourdb` with your PostgreSQL credentials.

### 3. Install Dependencies

```bash
go mod tidy
```

### 4. Run the Service

```bash
go run ./cmd/server
```

The service starts on `http://localhost:3000` (default port).

---

## 📡 API Endpoints

| Method  | Endpoint            | Description                          |
|---------|---------------------|--------------------------------------|
| `POST`  | `/urls`             | Register a new URL for monitoring    |
| `GET`   | `/urls`             | List all monitored URLs              |
| `PATCH` | `/urls/{id}`        | Update health status of a URL        |

### POST /urls
Register a new URL to monitor.

**Request Body:**
```json
{
  "address": "https://example.com"
}
```

**Note:** Health checks currently use `GET` by default. Support for other HTTP methods is planned.

**Response:**
```json
{
  "id": "e2c1b7f4-6d04-4fc6-a1de-2cf85801f645",
  "address": "https://example.com",
  "status": "unknown",
  "checked_at": "2025-07-21T12:05:07Z"
}
```
**Status:** `201 Created`

### GET /urls
List all monitored URLs.

**Response:**
```json
[
  {
    "id": "e2c1b7f4-6d04-4fc6-a1de-2cf85801f645",
    "address": "https://example.com",
    "status": "up",
    "checked_at": "2025-07-21T12:00:00Z"
  }
]
```

### PATCH /urls/{id}
Update the health status of a URL (used internally for now).

**Request Body:**
```json
{
  "status": "down"
}
```

**Response:**
```json
{
  "id": "e2c1b7f4-6d04-4fc6-a1de-2cf85801f645",
  "address": "https://example.com",
  "status": "down",
  "checked_at": "2025-07-21T12:05:30Z"
}
```
**Status:** `200 OK`

---

## 🧪 Testing with cURL

Test the API using the following cURL commands:

```bash
# Add a URL
curl -X POST http://localhost:3000/urls \
  -H "Content-Type: application/json" \
  -d '{"address": "https://example.com"}'

# List URLs
curl http://localhost:3000/urls

# Update status
curl -X PATCH http://localhost:3000/urls/e2c1b7f4-6d04-4fc6-a1de-2cf85801f645 \
  -H "Content-Type: application/json" \
  -d '{"status": "up"}'
```

---

## 🏛️ Architecture Overview

HCaaS follows a **clean architecture** with three layers:

- **Handler**: Manages HTTP requests and responses using Chi.
- **Service**: Handles business logic for URL registration and health checks.
- **Storage**: Persists data to PostgreSQL via `pgxpool`.

This modular design ensures testability and scalability.

---

## ✅ Design Principles

- **Idiomatic Go**: Simple, readable, and maintainable code.
- **Separation of Concerns**: Handlers delegate to services, which interact with storage.
- **Interface-Driven**: Enables easy mocking for testing.
- **Extensible**: Designed for future features like cron-based checks.

---

## 🧭 Roadmap

- [x] PostgreSQL integration
- [x] RESTful API with Chi
- [ ] Automated cron-based health checks
- [ ] Support for additional HTTP methods
- [ ] Alerts (email, webhooks)
- [ ] Authentication and rate limiting
- [ ] Comprehensive test suite with mocks

---

## 👨‍💻 Author

**Samiul Sk**  
*Senior Software Engineer*  
Kolkata, India  
Passionate about microservices, Go, and scalable system design.  
[LinkedIn](https://www.linkedin.com/in/samiul-sk/) | [GitHub](https://github.com/samims)

---

## 📜 License

This project is licensed under the [MIT License](LICENSE).

---

## 📌 Contributing

Contributions are welcome! Please:
1. Fork the repository.
2. Create a feature branch (`git checkout -b feature/xyz`).
3. Submit a pull request with clear descriptions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for details. Report issues or suggest features on [GitHub](https://github.com/kernelshard/hcaas).

*Want to enhance this project? Let me know if you’d like:*
- A `Dockerfile` and `docker-compose.yml` for containerized deployment
- A Mermaid diagram illustrating the architecture
- A `docs/` folder structure for GitHub Pages
- Additional badges (e.g., code coverage, last commit)

---

⭐️ Feel free to fork, improve, or use this as a portfolio project!
