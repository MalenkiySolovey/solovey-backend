package entitlement

import (
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/MalenkiySolovey/solovey-backend/storage/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func fixture(t *testing.T, client model.Client) (*gorm.DB, uint) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "clients.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.AutoMigrate(&model.Client{}, &model.Changes{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	return db, client.Id
}

func TestOwnerRenewsOnlyRecoverableDisableReasons(t *testing.T) {
	for _, reason := range []string{"", model.ClientDisabledExpired, model.ClientDisabledQuota, model.ClientDisabledManual, model.ClientDisabledLegacy, "FUTURE_SECURITY_DISABLE"} {
		t.Run("reason="+reason, func(t *testing.T) {
			db, id := fixture(t, model.Client{Name: `quoted "client"`, AccessDisableReason: reason, Expiry: 200, Volume: 100, Up: 5, Down: 7, TotalUp: 100, TotalDown: 200, Inbounds: json.RawMessage(`[2]`)})
			err := db.Transaction(func(tx *gorm.DB) error {
				result, err := Renew(tx, id, Terms{Days: 1, TrafficBytes: 10}, 100, "fixture")
				if err != nil {
					return err
				}
				if result.Usage != (Usage{5, 7}) || len(result.InboundIDs) != 1 || result.InboundIDs[0] != 2 {
					t.Errorf("renewal owner snapshot: %+v", result)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			var got model.Client
			if err := db.First(&got, id).Error; err != nil {
				t.Fatal(err)
			}
			allow := reason == "" || reason == model.ClientDisabledExpired || reason == model.ClientDisabledQuota
			wantReason := reason
			if allow {
				wantReason = ""
			}
			if got.Enable != allow || got.AccessDisableReason != wantReason || got.Expiry != 86600 || got.Volume != 110 || got.Up != 0 || got.Down != 0 || got.TotalUp != 105 || got.TotalDown != 207 {
				t.Fatalf("incorrect canonical Client: %+v", got)
			}
			var audit model.Changes
			if err := db.First(&audit).Error; err != nil || audit.Action != "renew" || audit.Actor != "fixture" || string(audit.Obj) != `"quoted \"client\""` {
				t.Fatalf("owner audit differs: %+v, %v", audit, err)
			}
		})
	}
}

func TestOwnerDoesNotCommitAndRollsBackOnAuditFailure(t *testing.T) {
	db, id := fixture(t, model.Client{Name: "refund", Enable: true, Expiry: 400, Volume: 500, Up: 30, Down: 40, TotalUp: 200, TotalDown: 300})
	if _, err := Renew(db, id, Terms{Days: 1}, 100, "fixture"); err == nil {
		t.Fatal("direct write acquired an independent transaction")
	}
	if _, err := Revoke(db, id, Terms{TrafficBytes: 100}, Usage{}, false, 100, "fixture"); err == nil {
		t.Fatal("direct refund acquired an independent transaction")
	}
	auditFailure := errors.New("injected audit failure")
	if err := db.Callback().Create().Before("gorm:create").Register("test:auditFailure", func(tx *gorm.DB) {
		if tx.Statement.Table == "changes" {
			_ = tx.AddError(auditFailure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := Revoke(tx, id, Terms{Days: 1, TrafficBytes: 100}, Usage{10, 20}, false, 100, "fixture")
		return err
	}); !errors.Is(err, auditFailure) {
		t.Fatalf("unexpected refund failure: %v", err)
	}
	var got model.Client
	if err := db.First(&got, id).Error; err != nil {
		t.Fatal(err)
	}
	if !got.Enable || got.Volume != 500 || got.Expiry != 400 || got.Up != 30 || got.Down != 40 || got.TotalUp != 200 || got.TotalDown != 300 {
		t.Fatalf("audit failure committed a partial refund: %+v", got)
	}
	if err := db.Callback().Create().Remove("test:auditFailure"); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := Revoke(tx, id, Terms{Days: 1, TrafficBytes: 100}, Usage{10, 20}, false, 100, "fixture")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&got, id).Error; err != nil {
		t.Fatal(err)
	}
	if !got.Enable || got.Volume != 400 || got.Expiry != 100 || got.Up != 30 || got.Down != 40 || got.TotalUp != 190 || got.TotalDown != 280 {
		t.Fatalf("refund changed policy or newer usage: %+v", got)
	}
}

func TestOwnerRejectsOverflow(t *testing.T) {
	db, id := fixture(t, model.Client{Name: "limit", Volume: math.MaxInt64, Expiry: math.MaxInt64})
	for _, terms := range []Terms{{Days: 1}, {TrafficBytes: 1}} {
		if err := db.Transaction(func(tx *gorm.DB) error {
			_, err := Renew(tx, id, terms, 100, "fixture")
			return err
		}); err == nil {
			t.Fatalf("overflow accepted: %+v", terms)
		}
	}
}
