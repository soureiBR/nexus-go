// internal/storage/sql_storage.go
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"yourproject/pkg/logger"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// SQLStore implements the storage for sessions using SQLite
type SQLStore struct {
	dbPath      string
	dbContainer *sqlstore.Container
	waLogger    waLog.Logger
	db          *sql.DB
}

// UserDeviceMapping represents the mapping between a userID and a deviceJID
type UserDeviceMapping struct {
	UserID    string
	DeviceJID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewSQLStore creates a new SQL-based storage
func NewSQLStore(dbPath string) (*SQLStore, error) {
	// Ensure directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Configure WhatsApp logger
	waLogger := waLog.Stdout("whatsapp", "INFO", true)

	// Create database container for whatsmeow
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+dbPath+"?_foreign_keys=on", waLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to whatsmeow database: %w", err)
	}

	// Open connection for our own tables
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		container.Close()
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLStore{
		dbPath:      dbPath,
		dbContainer: container,
		waLogger:    waLogger,
		db:          db,
	}

	// Initialize custom tables
	if err := store.initTables(); err != nil {
		container.Close()
		db.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return store, nil
}

// initTables creates the necessary tables in the database
func (s *SQLStore) initTables() error {
	// Table for mapping between userID and deviceJID
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_device_mapping (
			user_id TEXT PRIMARY KEY,
			device_jid TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create user_device_mapping table: %w", err)
	}

	return nil
}

// SaveUserDeviceMapping saves the mapping between userID and deviceJID
func (s *SQLStore) SaveUserDeviceMapping(userID, deviceJID string) error {
	now := time.Now()
	_, err := s.db.Exec(`
		INSERT INTO user_device_mapping (user_id, device_jid, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			device_jid = excluded.device_jid,
			updated_at = excluded.updated_at
	`, userID, deviceJID, now, now)

	if err != nil {
		return fmt.Errorf("failed to save userID->deviceJID mapping: %w", err)
	}

	logger.Debug("Mapping saved", "user_id", userID, "device_jid", deviceJID)
	return nil
}

// GetDeviceJIDByUserID gets the deviceJID associated with the userID
func (s *SQLStore) GetDeviceJIDByUserID(userID string) (string, error) {
	var deviceJID string
	err := s.db.QueryRow(`
		SELECT device_jid FROM user_device_mapping
		WHERE user_id = ?
	`, userID).Scan(&deviceJID)

	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("mapping not found for userID: %s", userID)
		}
		return "", fmt.Errorf("failed to query deviceJID: %w", err)
	}

	return deviceJID, nil
}

// GetAllUserDeviceMappings returns all userID -> deviceJID mappings
func (s *SQLStore) GetAllUserDeviceMappings() ([]UserDeviceMapping, error) {
	rows, err := s.db.Query(`
		SELECT user_id, device_jid, created_at, updated_at
		FROM user_device_mapping
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query mappings: %w", err)
	}
	defer rows.Close()

	var mappings []UserDeviceMapping
	for rows.Next() {
		var mapping UserDeviceMapping
		var createdAt, updatedAt string

		if err := rows.Scan(&mapping.UserID, &mapping.DeviceJID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to read mapping: %w", err)
		}

		// Convert strings to time.Time
		mapping.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		mapping.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		mappings = append(mappings, mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during result iteration: %w", err)
	}

	return mappings, nil
}

// DeleteUserDeviceMapping removes a mapping
func (s *SQLStore) DeleteUserDeviceMapping(userID string) error {
	_, err := s.db.Exec(`
		DELETE FROM user_device_mapping
		WHERE user_id = ?
	`, userID)

	if err != nil {
		return fmt.Errorf("failed to remove mapping: %w", err)
	}

	logger.Debug("Mapping removed", "user_id", userID)
	return nil
}

// GetDBContainer returns the database container for whatsmeow
func (s *SQLStore) GetDBContainer() *sqlstore.Container {
	return s.dbContainer
}

// Close closes all database connections
func (s *SQLStore) Close() error {
	var errs []error

	if err := s.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close SQL connection: %w", err))
	}

	if err := s.dbContainer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close whatsmeow container: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing SQLStore: %v", errs)
	}

	return nil
}
