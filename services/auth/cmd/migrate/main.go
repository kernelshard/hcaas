package main

import (
	"log"
	"os"

	"fmt"
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
		log.Println("Migration up done")
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
		log.Println("Migration down done")
	default:
		log.Fatal("unknown command, verify Makefile")
	}
}
