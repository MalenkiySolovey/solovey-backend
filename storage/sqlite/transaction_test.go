package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunTransactionPreservesScopeAndOutermostRollback(t *testing.T) {
	db, err := gorm.Open(gormsqlite.Open(filepath.Join(t.TempDir(), "tx.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.Exec("CREATE TABLE fixture (value INTEGER NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}

	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "admitted")
	abort := errors.New("reject operation before commit")
	write := func(outer *gorm.DB) error {
		return RunTransaction(outer, func(inner *gorm.DB) error {
			if inner.Statement.Context.Value(key{}) != "admitted" {
				t.Fatal("nested write lost the caller's scope")
			}
			return inner.Exec("INSERT INTO fixture (value) VALUES (1)").Error
		})
	}

	if err := RunTransaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := write(tx); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatalf("outer rollback: %v", err)
	}
	var rows int64
	if err := db.Table("fixture").Count(&rows).Error; err != nil || rows != 0 {
		t.Fatalf("a nested write escaped rollback: rows=%d err=%v", rows, err)
	}
	if err := RunTransaction(db.WithContext(ctx), write); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("fixture").Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("outer commit did not persist the write: rows=%d err=%v", rows, err)
	}
}
