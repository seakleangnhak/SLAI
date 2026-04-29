package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var ErrInvalidAuditLogFilter = errors.New("invalid audit log filter")

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

type AuditLogRepository struct {
	db platformdb.Executor
}

func NewAuditLogRepository(db platformdb.Executor) AuditLogRepository {
	return AuditLogRepository{db: db}
}

type AuditLogFilter struct {
	AdminID    string
	Action     string
	TargetType string
	TargetID   string
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

type AuditLogItem struct {
	ID         string         `json:"id"`
	AdminID    string         `json:"admin_id"`
	AdminEmail string         `json:"admin_email"`
	Action     string         `json:"action"`
	TargetType *string        `json:"target_type"`
	TargetID   *string        `json:"target_id"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AuditLogListResult struct {
	Items  []AuditLogItem `json:"items"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
	Total  int64          `json:"total"`
}

func (r AuditLogRepository) List(ctx context.Context, filter AuditLogFilter) (AuditLogListResult, error) {
	filter = normalizeAuditLogFilter(filter)
	if err := validateAuditLogFilter(filter); err != nil {
		return AuditLogListResult{}, err
	}

	where, args := auditLogWhere(filter)
	countQuery := `
		SELECT count(*)
		FROM admin_audit_logs aal
		JOIN users u ON u.id = aal.admin_id
	` + where

	var total int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return AuditLogListResult{}, fmt.Errorf("count audit logs: %w", err)
	}

	args = append(args, filter.Limit, filter.Offset)
	query := `
		SELECT aal.id::text, aal.admin_id::text, u.email, aal.action, aal.target_type,
		       aal.target_id, aal.metadata, aal.created_at
		FROM admin_audit_logs aal
		JOIN users u ON u.id = aal.admin_id
	` + where + `
		ORDER BY aal.created_at DESC, aal.id DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return AuditLogListResult{}, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	items := []AuditLogItem{}
	for rows.Next() {
		var item AuditLogItem
		var metadataBytes []byte
		if err := rows.Scan(
			&item.ID,
			&item.AdminID,
			&item.AdminEmail,
			&item.Action,
			&item.TargetType,
			&item.TargetID,
			&metadataBytes,
			&item.CreatedAt,
		); err != nil {
			return AuditLogListResult{}, fmt.Errorf("scan audit log: %w", err)
		}
		item.Metadata = map[string]any{}
		if len(metadataBytes) > 0 {
			if err := json.Unmarshal(metadataBytes, &item.Metadata); err != nil {
				return AuditLogListResult{}, fmt.Errorf("decode audit metadata: %w", err)
			}
			item.Metadata = sanitizeAuditMetadataMap(item.Metadata)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return AuditLogListResult{}, fmt.Errorf("iterate audit logs: %w", err)
	}

	return AuditLogListResult{Items: items, Limit: filter.Limit, Offset: filter.Offset, Total: total}, nil
}

func normalizeAuditLogFilter(filter AuditLogFilter) AuditLogFilter {
	filter.AdminID = strings.TrimSpace(filter.AdminID)
	filter.Action = strings.TrimSpace(filter.Action)
	filter.TargetType = strings.TrimSpace(filter.TargetType)
	filter.TargetID = strings.TrimSpace(filter.TargetID)
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

func validateAuditLogFilter(filter AuditLogFilter) error {
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return ErrInvalidAuditLogFilter
	}
	return nil
}

func auditLogWhere(filter AuditLogFilter) (string, []any) {
	args := []any{}
	clauses := []string{}

	if filter.AdminID != "" {
		args = append(args, filter.AdminID)
		clauses = append(clauses, fmt.Sprintf("aal.admin_id::text = $%d", len(args)))
	}
	if filter.Action != "" {
		args = append(args, filter.Action)
		clauses = append(clauses, fmt.Sprintf("aal.action = $%d", len(args)))
	}
	if filter.TargetType != "" {
		args = append(args, filter.TargetType)
		clauses = append(clauses, fmt.Sprintf("aal.target_type = $%d", len(args)))
	}
	if filter.TargetID != "" {
		args = append(args, filter.TargetID)
		clauses = append(clauses, fmt.Sprintf("aal.target_id = $%d", len(args)))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		clauses = append(clauses, fmt.Sprintf("aal.created_at >= $%d", len(args)))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		clauses = append(clauses, fmt.Sprintf("aal.created_at <= $%d", len(args)))
	}

	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func sanitizeAuditMetadataMap(metadata map[string]any) map[string]any {
	sanitized := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if isSensitiveAuditMetadataKey(key) {
			sanitized[key] = "[redacted]"
			continue
		}
		sanitized[key] = sanitizeAuditMetadataValue(value)
	}
	return sanitized
}

func sanitizeAuditMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeAuditMetadataMap(typed)
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, sanitizeAuditMetadataValue(item))
		}
		return items
	default:
		return value
	}
}

func isSensitiveAuditMetadataKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	sensitiveFragments := []string{
		"password",
		"token",
		"secret",
		"pepper",
		"raw_api_key",
		"raw_key",
		"key_hash",
		"management_token",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
