package sqlite

import (
	"errors"

	"gorm.io/gorm"
)

// RunTransaction commits one Panel operation on its admitted connection.
// An operation already inside a transaction retains the same connection and
// GORM savepoint semantics; it cannot commit independently of its caller.
func RunTransaction(db *gorm.DB, work func(tx *gorm.DB) error) error {
	if db == nil || work == nil {
		return errors.New("database and transaction callback are required")
	}
	if db.Error != nil {
		return db.Error
	}
	return db.Transaction(work)
}
