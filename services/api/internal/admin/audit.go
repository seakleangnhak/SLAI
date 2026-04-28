package admin

import (
	"context"
	"encoding/json"
	"fmt"

	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

type AuditLogger struct {
	db platformdb.Executor
}

func NewAuditLogger(db platformdb.Executor) AuditLogger {
	return AuditLogger{db: db}
}

func (l AuditLogger) Log(ctx context.Context, adminID, action string, targetType *string, targetID *string, metadata map[string]any) error {
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	if metadata == nil {
		metadataBytes = nil
	}

	_, err = l.db.Exec(ctx, `
		INSERT INTO admin_audit_logs (admin_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`, adminID, action, targetType, targetID, metadataBytes)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}
