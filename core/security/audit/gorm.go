package audit

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/jsonx"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type GormWriter struct {
	db *gorm.DB
}

func NewGormWriter(db *gorm.DB) *GormWriter {
	return &GormWriter{db: db}
}

func (w *GormWriter) Write(ctx context.Context, event *Event) error {
	if event == nil {
		return ctx.Err()
	}
	return w.WriteBatch(ctx, []*Event{event})
}

func (w *GormWriter) WriteBatch(ctx context.Context, events []*Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w.db == nil || len(events) == 0 {
		return nil
	}
	rows := make([]securityEventRow, 0, len(events))
	now := time.Now().Unix()
	for _, event := range events {
		if event == nil {
			continue
		}
		reason, err := marshalJSON(event.Reason)
		if err != nil {
			return err
		}
		extra, err := marshalJSON(event.Extra)
		if err != nil {
			return err
		}
		rows = append(rows, securityEventRow{
			Scene:     event.Scene,
			Event:     event.Event,
			Level:     event.Level,
			UID:       event.UID,
			IP:        event.IP,
			DeviceID:  event.DeviceID,
			Score:     event.Score,
			Action:    event.Action,
			Reason:    reason,
			Extra:     extra,
			CreatedAt: now,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	table := w.db.NamingStrategy.TableName("security_event")
	return w.db.WithContext(ctx).Table(table).Create(&rows).Error
}

type securityEventRow struct {
	ID        int64  `gorm:"primaryKey"`
	Scene     string `gorm:"size:64;index"`
	Event     string `gorm:"size:64;index"`
	Level     string `gorm:"size:32;index"`
	UID       int64  `gorm:"index"`
	IP        string `gorm:"size:64;index"`
	DeviceID  string `gorm:"size:128;index"`
	Score     int
	Action    string         `gorm:"size:64"`
	Reason    datatypes.JSON `gorm:"type:jsonb"`
	Extra     datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt int64          `gorm:"index"`
}

func marshalJSON(v map[string]any) (datatypes.JSON, error) {
	if len(v) == 0 {
		return datatypes.JSON([]byte("{}")), nil
	}
	b, err := jsonx.Marshal(v)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(b), nil
}
