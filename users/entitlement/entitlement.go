// Package entitlement mutates the Panel's canonical Client quota and expiry.
// Payment authentication, order state, and commit belong to the caller.
package entitlement

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/MalenkiySolovey/solovey-backend/storage/model"
	"gorm.io/gorm"
)

type Terms struct {
	Days         int
	TrafficBytes int64
}

type Usage struct {
	Up, Down int64
}

type Renewal struct {
	Usage      Usage
	InboundIDs []uint
}

// Renew joins the caller's admitted write transaction. Usage is the previous
// live window, to be recorded in the same transaction for a possible refund.
func Renew(tx *gorm.DB, clientID uint, terms Terms, now int64, actor string) (Renewal, error) {
	client, err := load(tx, clientID, terms)
	if err != nil {
		return Renewal{}, err
	}
	// Evaluate access policy when the write runs. A renewal cannot override
	// manual, security, legacy, or future disable reasons.
	updates := map[string]any{
		"enable":                gorm.Expr("CASE WHEN access_disable_reason = '' OR access_disable_reason IN (?, ?) THEN true ELSE false END", model.ClientDisabledQuota, model.ClientDisabledExpired),
		"access_disable_reason": gorm.Expr("CASE WHEN access_disable_reason IN (?, ?) THEN '' ELSE access_disable_reason END", model.ClientDisabledQuota, model.ClientDisabledExpired),
	}
	if terms.Days > 0 {
		expiry, err := add(max(client.Expiry, now), int64(terms.Days)*86400)
		if err != nil {
			return Renewal{}, fmt.Errorf("client entitlement expiry: %w", err)
		}
		updates["expiry"] = expiry
	}
	var renewal Renewal
	if terms.TrafficBytes > 0 {
		for _, counter := range []struct {
			name        string
			base, delta int64
		}{
			{"volume", client.Volume, terms.TrafficBytes},
			{"total_up", client.TotalUp, client.Up},
			{"total_down", client.TotalDown, client.Down},
		} {
			value, err := add(counter.base, counter.delta)
			if err != nil {
				return Renewal{}, fmt.Errorf("client entitlement %s: %w", counter.name, err)
			}
			updates[counter.name] = value
		}
		updates["up"], updates["down"] = int64(0), int64(0)
		renewal.Usage = Usage{Up: client.Up, Down: client.Down}
	}
	renewal.InboundIDs, err = write(tx, client, updates, now, actor, "renew")
	if err != nil {
		return Renewal{}, err
	}
	return renewal, nil
}

// Revoke reverses the immutable grant. The caller determines whether a newer
// order owns the live usage window; access policy is never changed by refund.
func Revoke(tx *gorm.DB, clientID uint, terms Terms, granted Usage, restoreLiveUsage bool, now int64, actor string) ([]uint, error) {
	client, err := load(tx, clientID, terms)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if terms.Days > 0 && client.Expiry > 0 {
		updates["expiry"] = max(client.Expiry-int64(terms.Days)*86400, now)
	}
	if terms.TrafficBytes > 0 {
		if granted.Up < 0 || granted.Down < 0 {
			return nil, errors.New("invalid client entitlement usage snapshot")
		}
		updates["volume"] = subtract(client.Volume, terms.TrafficBytes)
		updates["total_up"] = subtract(client.TotalUp, granted.Up)
		updates["total_down"] = subtract(client.TotalDown, granted.Down)
		if restoreLiveUsage {
			updates["up"], updates["down"] = granted.Up, granted.Down
		}
	}
	if len(updates) == 0 {
		return nil, nil
	}
	return write(tx, client, updates, now, actor, "refund")
}

func load(tx *gorm.DB, id uint, terms Terms) (model.Client, error) {
	var client model.Client
	if tx == nil || tx.Statement == nil {
		return client, errors.New("client entitlement requires a transaction")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return client, errors.New("client entitlement requires a transaction")
	}
	if id == 0 || terms.Days < 0 || int64(terms.Days) > math.MaxInt64/86400 || terms.TrafficBytes < 0 {
		return client, errors.New("invalid client entitlement input")
	}
	err := tx.Select("id", "name", "inbounds", "expiry", "volume", "up", "down", "total_up", "total_down").First(&client, id).Error
	return client, err
}

func write(tx *gorm.DB, client model.Client, updates map[string]any, now int64, actor, action string) ([]uint, error) {
	result := tx.Model(&model.Client{}).Where("id = ?", client.Id).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, errors.New("client entitlement owner no longer exists")
	}
	name, _ := json.Marshal(client.Name)
	if err := tx.Create(&model.Changes{DateTime: now, Actor: actor, Key: "clients", Action: action, Obj: name}).Error; err != nil {
		return nil, err
	}
	var inbounds []uint
	_ = json.Unmarshal(client.Inbounds, &inbounds)
	return inbounds, nil
}

func add(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left > math.MaxInt64-right {
		return 0, errors.New("value would overflow")
	}
	return left + right, nil
}

func subtract(current, granted int64) int64 {
	return max(current, granted) - granted
}
