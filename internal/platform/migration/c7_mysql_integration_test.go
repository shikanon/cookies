package migration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestC7CreativeVersionMigrationRecoversAfterTrackingLoss(t *testing.T) {
	dsn := os.Getenv("COOKIES_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("COOKIES_TEST_MYSQL_DSN is not configured")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const migrationID = "migrations/creative/20260811171000_creative_edit_versions.up.sql"
	if _, err := db.ExecContext(ctx, "DELETE FROM platform_schema_migrations WHERE migration_id = ?", migrationID); err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(workingDirectory) }()
	if err := Run(ctx, db, "migrations"); err != nil {
		t.Fatalf("rerun after lost tracking record: %v", err)
	}
	var columns, constraints int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND COLUMN_NAME = 'edit_task_id' AND TABLE_NAME IN ('creative_versions', 'creative_packages')`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND CONSTRAINT_NAME IN ('fk_creative_versions_edit_task', 'fk_creative_packages_edit_task', 'chk_creative_versions_single_source')`).Scan(&constraints); err != nil {
		t.Fatal(err)
	}
	if columns != 2 || constraints != 3 {
		t.Fatalf("recovered schema columns=%d constraints=%d", columns, constraints)
	}
}
