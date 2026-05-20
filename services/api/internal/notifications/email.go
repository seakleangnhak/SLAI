package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/auth"
	"github.com/slai/slai/services/api/internal/ledger"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

type Config struct {
	Enabled        bool
	ThresholdUnits int64
}

type Service struct {
	db     *pgxpool.Pool
	sender auth.EmailSender
	cfg    Config
}

func NewService(db *pgxpool.Pool, sender auth.EmailSender, cfg Config) Service {
	if sender == nil {
		sender = auth.NoopEmailSender{}
	}
	return Service{db: db, sender: sender, cfg: cfg}
}

func (s Service) MaybeSendLowBalanceAlert(ctx context.Context, balance ledger.Balance) error {
	if !s.cfg.Enabled || s.cfg.ThresholdUnits <= 0 || balance.AvailableUnits > s.cfg.ThresholdUnits {
		return nil
	}

	alertKey := lowBalanceAlertKey(s.cfg.ThresholdUnits)
	var email string
	var shouldSend bool
	if err := platformdb.InTx(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT u.email,
			       NOT EXISTS (
			           SELECT 1
			           FROM user_email_notifications n
			           WHERE n.user_id = u.id
			             AND n.kind = $2
			             AND n.alert_key = $3
			       )
			FROM users u
			WHERE u.id = $1
			  AND u.status = 'ACTIVE'
			FOR UPDATE
		`, balance.UserID, KindLowBalance, alertKey).Scan(&email, &shouldSend)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load low balance notification state: %w", err)
		}
		if !shouldSend {
			return nil
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO user_email_notifications (user_id, kind, alert_key, sent_at)
			VALUES ($1, $2, $3, $4)
		`, balance.UserID, KindLowBalance, alertKey, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("store low balance notification state: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if !shouldSend || email == "" {
		return nil
	}
	if err := s.sender.SendLowBalanceAlert(ctx, email, balance.AvailableUnits, s.cfg.ThresholdUnits); err != nil {
		if _, deleteErr := s.db.Exec(ctx, `
			DELETE FROM user_email_notifications
			WHERE user_id = $1
			  AND kind = $2
			  AND alert_key = $3
		`, balance.UserID, KindLowBalance, alertKey); deleteErr != nil {
			return fmt.Errorf("send low balance alert: %w; release low balance retry state: %v", err, deleteErr)
		}
		return fmt.Errorf("send low balance alert: %w", err)
	}
	return nil
}

func lowBalanceAlertKey(thresholdUnits int64) string {
	return fmt.Sprintf("threshold:%d", thresholdUnits)
}
