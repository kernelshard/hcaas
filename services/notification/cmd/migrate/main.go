package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	// Load .env file
	_ = os.Setenv("DB_HOST", "localhost")
	_ = os.Setenv("DB_PORT", "5432")
	_ = os.Setenv("DB_USER", "hcaas_user")
	_ = os.Setenv("DB_PASSWORD", "hcaas_password")
	_ = os.Setenv("DB_NAME", "hcaas_db")

	// Create migrations directory if it doesn't exist
	if err := os.MkdirAll("migrations", 0755); err != nil {
		log.Fatal(err)
	}

	cmd := os.Args[len(os.Args)-1]

	// Handle create command separately as it doesn't need DB connection
	if len(os.Args) > 2 && os.Args[1] == "create" {
		name := os.Args[2]
		if name == "" {
			log.Fatal("migration name is required")
		}
		// timestamp format: YYYYMMDDHHMMSS
		timestamp := time.Now().Format("20060102150405")
		base := fmt.Sprintf("%s_%s", timestamp, name)

		up := filepath.Join("migrations", base+".up.sql")
		down := filepath.Join("migrations", base+".down.sql")

		if err := os.WriteFile(up, []byte{}, 0644); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(down, []byte{}, 0644); err != nil {
			log.Fatal(err)
		}

		log.Printf("Created migration files:\n%s\n%s", up, down)
		return
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL environment variable is required")
	}

	m, err := migrate.New(
		"file://migrations",
		dbURL,
	)
	if err != nil {
		log.Fatal(err)
	}

	switch cmd {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
		log.Println("Migrations applied successfully")
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
		log.Println("Migrations rolled back successfully")
	default:
		log.Fatalf("Unknown command: %s", cmd)
	}
}

func atoi(s string) int {
	var i int
	_, err := fmt.Sscan(s, &i)
	if err != nil {
		log.Fatal(err)
	}
	return i
}
