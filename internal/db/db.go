// db.go
package db

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sukalov/mshkbot/internal/utils"
)

var (
	Database *gorm.DB
	once     sync.Once
	initErr  error
)

func init() {
	once.Do(func() {
		env, err := utils.LoadEnv([]string{"TURSO_DATABASE_URL", "TURSO_AUTH_TOKEN"})
		if err != nil {
			initErr = fmt.Errorf("failed to load db env: %w", err)
			log.Fatalf("database initialization failed: %v", initErr)
		}
		url := fmt.Sprintf("%s?authToken=%s", env["TURSO_DATABASE_URL"], env["TURSO_AUTH_TOKEN"])

		// open connection with database/sql first
		sqlDB, err := sql.Open("libsql", url)
		if err != nil {
			log.Fatalf("failed to open db: %v", err)
		}

		// connection pool configuration
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(25)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)

		// verifying database connection
		if pingErr := sqlDB.Ping(); pingErr != nil {
			log.Fatalf("failed to ping database: %v", pingErr)
		}

		// wrap with gorm
		Database, initErr = gorm.Open(sqlite.Dialector{
			Conn: sqlDB,
		}, &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
			NowFunc: func() time.Time {
				return time.Now().UTC()
			},
		})
		if initErr != nil {
			log.Fatalf("failed to initialize gorm: %v", initErr)
		}

		// run auto migrations
		if err := Database.AutoMigrate(
			&User{},
			// add other models here as you create them
		); err != nil {
			log.Fatalf("failed to auto migrate: %v", err)
		}

		log.Println("database connected and schema migrated successfully")
	})

}

// closes the database connection safely
func Close() {
	if Database != nil {
		sqlDB, err := Database.DB()
		if err != nil {
			log.Printf("error getting sql.DB: %v", err)
			return
		}
		if err := sqlDB.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}
}
