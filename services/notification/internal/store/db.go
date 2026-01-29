package store

import (
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kernelshard/hcaas/services/notification/internal/config"
)

// ConnectPostgres creates and returns a *sql.DB connection
func ConnectPostgres(dbCfg config.DBConfig) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", dbCfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}

	db.SetMaxOpenConns(dbCfg.MaxOpenConn)
	db.SetConnMaxIdleTime(dbCfg.ConnMaxIdle)

	// ping to ensure connection is valid
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}
	return db, nil
}
