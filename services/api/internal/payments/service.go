package payments

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/admin"
	"github.com/slai/slai/services/api/internal/ledger"
	"github.com/slai/slai/services/api/internal/packages"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	"github.com/slai/slai/services/api/internal/slaipayment"
)

var (
	ErrInvalidTopUp              = errors.New("invalid manual top-up")
	ErrIdempotencyConflict       = errors.New("idempotency key belongs to a different manual top-up")
	ErrInvalidPaymentSettings    = errors.New("invalid payment settings")
	ErrPaymentSettingsDisabled   = errors.New("bakong khqr checkout is disabled")
	ErrPaymentSettingsIncomplete = errors.New("bakong khqr checkout is not fully configured")
	ErrPaymentNotFound           = errors.New("payment not found")
	ErrPaymentForbidden          = errors.New("payment does not belong to user")
	ErrInvalidPaymentState       = errors.New("invalid payment state")
	ErrInvalidPaymentProof       = errors.New("invalid payment proof")
	ErrPaymentReferenceRequired  = errors.New("payment reference is required")
	ErrDuplicatePaymentReference = errors.New("payment reference already used")
	ErrPaymentAlreadyPaid        = errors.New("payment already paid")
	ErrPaymentProviderDisabled   = errors.New("automatic payment provider is disabled")
	ErrPaymentProviderMismatch   = errors.New("payment provider response does not match local payment")
	ErrPaymentCallbackInvalid    = errors.New("invalid payment callback")
)

type ProviderConfig struct {
	SLAIPaymentEnabled         bool
	SLAIPaymentCallbackBaseURL string
	SLAIPaymentMerchantPrefix  string
	SLAIPaymentDefaultExpiry   time.Duration
}

type AutoResumeAPIKeyFunc func(ctx context.Context, userID string, balance ledger.Balance)

type Service struct {
	pool              *pgxpool.Pool
	cfg               ProviderConfig
	slaiPaymentClient slaipayment.Client
	autoResumeAPIKey  AutoResumeAPIKeyFunc
}

func NewService(pool *pgxpool.Pool, cfgs ...ProviderConfig) Service {
	cfg := ProviderConfig{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	return Service{pool: pool, cfg: cfg}
}

func (s Service) WithSLAIPaymentClient(client slaipayment.Client) Service {
	s.slaiPaymentClient = client
	return s
}

func (s Service) WithAutoResumeAPIKey(fn AutoResumeAPIKeyFunc) Service {
	s.autoResumeAPIKey = fn
	return s
}

func (s Service) autoResumeAfterCredit(ctx context.Context, userID string, entry ledger.Entry, balance ledger.Balance) {
	if s.autoResumeAPIKey == nil || balance.AvailableUnits <= 0 {
		return
	}
	balanceBeforeCredit := entry.BalanceAfterUnits - entry.DeltaUnits
	if balanceBeforeCredit > 0 {
		return
	}
	s.autoResumeAPIKey(ctx, userID, balance)
}

func (s Service) ListForUser(ctx context.Context, userID string, limit int, offset int) ([]Payment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT p.id::text, p.user_id::text, p.package_id::text, cp.name, p.provider, p.provider_ref,
		       p.amount_minor, p.currency, p.credit_units, p.status, p.admin_id::text, p.note,
		       EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = p.id),
		       p.rejection_reason,
		       CASE WHEN p.status = 'paid' THEN p.admin_payment_reference ELSE NULL END,
		       p.reviewed_by_admin_id::text, p.reviewed_at, p.created_at, p.updated_at, p.paid_at,
		       p.external_payment_id, p.checkout_reference, p.qr_payload, p.qr_image_data_uri, p.qr_md5,
		       p.expires_at, p.callback_received_at, p.provider_status, p.provider_transaction_id, p.provider_apv
		FROM payments p
		LEFT JOIN credit_packages cp ON cp.id = p.package_id
		WHERE p.user_id = $1
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list user payments: %w", err)
	}
	defer rows.Close()

	items := []Payment{}
	for rows.Next() {
		payment, err := scanPaymentDetail(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, payment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user payments: %w", err)
	}
	return items, nil
}

func (s Service) GetForUser(ctx context.Context, userID, paymentID string) (Payment, error) {
	payment, err := s.getPayment(ctx, paymentID)
	if err != nil {
		return Payment{}, err
	}
	if payment.UserID != userID {
		return Payment{}, ErrPaymentForbidden
	}
	return payment, nil
}

func (s Service) GetSettings(ctx context.Context, provider string) (PaymentSettings, error) {
	settings, err := scanPaymentSettings(s.pool.QueryRow(ctx, `
		SELECT id::text, provider, enabled, display_name, account_name, account_id,
		       khqr_image_path, khqr_image_mime, instructions, created_at, updated_at
		FROM payment_settings
		WHERE provider = $1
	`, provider))
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentSettings{}, ErrPaymentSettingsIncomplete
	}
	if err != nil {
		return PaymentSettings{}, err
	}
	settings.KHQRImageURL = khqrImageURL(settings)
	return settings, nil
}

func (s Service) UpdateSettings(ctx context.Context, adminID, provider string, input PaymentSettingsInput) (PaymentSettings, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.AccountName = cleanOptional(input.AccountName)
	input.AccountID = cleanOptional(input.AccountID)
	input.Instructions = cleanOptional(input.Instructions)
	if input.DisplayName == "" {
		return PaymentSettings{}, ErrInvalidPaymentSettings
	}
	if input.Enabled && (input.AccountName == nil || input.AccountID == nil) {
		return PaymentSettings{}, ErrInvalidPaymentSettings
	}

	var settings PaymentSettings
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		updated, err := scanPaymentSettings(tx.QueryRow(ctx, `
			UPDATE payment_settings
			SET enabled = $2,
			    display_name = $3,
			    account_name = $4,
			    account_id = $5,
			    instructions = $6
			WHERE provider = $1
			RETURNING id::text, provider, enabled, display_name, account_name, account_id,
			          khqr_image_path, khqr_image_mime, instructions, created_at, updated_at
		`, provider, input.Enabled, input.DisplayName, input.AccountName, input.AccountID, input.Instructions))
		if err != nil {
			return err
		}
		targetType := "payment_settings"
		targetID := updated.Provider
		if err := admin.NewAuditLogger(tx).Log(ctx, adminID, "payment_settings_updated", &targetType, &targetID, map[string]any{
			"provider": provider,
			"enabled":  updated.Enabled,
		}); err != nil {
			return err
		}
		settings = updated
		return nil
	})
	if err != nil {
		return PaymentSettings{}, err
	}
	settings.KHQRImageURL = khqrImageURL(settings)
	return settings, nil
}

func (s Service) UpdateKHQRImage(ctx context.Context, adminID, provider string, file StoredFile) (PaymentSettings, error) {
	if file.Path == "" || file.MIME == "" || file.Size <= 0 {
		return PaymentSettings{}, ErrInvalidPaymentSettings
	}
	var settings PaymentSettings
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		updated, err := scanPaymentSettings(tx.QueryRow(ctx, `
			UPDATE payment_settings
			SET khqr_image_path = $2,
			    khqr_image_mime = $3
			WHERE provider = $1
			RETURNING id::text, provider, enabled, display_name, account_name, account_id,
			          khqr_image_path, khqr_image_mime, instructions, created_at, updated_at
		`, provider, file.Path, file.MIME))
		if err != nil {
			return err
		}
		targetType := "payment_settings"
		targetID := updated.Provider
		if err := admin.NewAuditLogger(tx).Log(ctx, adminID, "payment_settings_khqr_uploaded", &targetType, &targetID, map[string]any{
			"provider": provider,
			"mime":     file.MIME,
			"size":     file.Size,
			"sha256":   file.SHA256,
		}); err != nil {
			return err
		}
		settings = updated
		return nil
	})
	if err != nil {
		return PaymentSettings{}, err
	}
	settings.KHQRImageURL = khqrImageURL(settings)
	return settings, nil
}

func (s Service) CheckoutPackage(ctx context.Context, userID, packageID string) (CheckoutResult, error) {
	if s.cfg.SLAIPaymentEnabled {
		return s.checkoutSLAIPayment(ctx, userID, packageID)
	}
	return s.checkoutManualProof(ctx, userID, packageID)
}

func (s Service) checkoutManualProof(ctx context.Context, userID, packageID string) (CheckoutResult, error) {
	settings, err := s.GetSettings(ctx, ProviderBakongKHQR)
	if err != nil {
		return CheckoutResult{}, err
	}
	if !settings.Enabled {
		return CheckoutResult{}, ErrPaymentSettingsDisabled
	}
	if settings.AccountName == nil || settings.AccountID == nil || settings.KHQRImagePath == nil || settings.KHQRImageURL == nil {
		return CheckoutResult{}, ErrPaymentSettingsIncomplete
	}

	pkg, err := packages.NewRepository(s.pool).Get(ctx, packageID)
	if err != nil {
		return CheckoutResult{}, err
	}
	if !pkg.Active {
		return CheckoutResult{}, packages.ErrInvalidPackage
	}

	var payment Payment
	err = platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		created, err := scanPayment(tx.QueryRow(ctx, `
			INSERT INTO payments (
				user_id, package_id, provider, amount_minor, currency, credit_units, status
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id::text, user_id::text, package_id::text, $8::text, provider, provider_ref,
			          amount_minor, currency, credit_units, status, admin_id::text, note,
			          false, rejection_reason, NULL::text, reviewed_by_admin_id::text, reviewed_at,
			          created_at, updated_at, paid_at
		`, userID, packageID, ProviderBakongKHQR, pkg.PriceMinor, strings.ToUpper(pkg.Currency), pkg.CreditUnits+pkg.BonusCreditUnits, StatusPendingProof, pkg.Name))
		if err != nil {
			return err
		}
		payment = created
		return nil
	})
	if err != nil {
		return CheckoutResult{}, err
	}

	return CheckoutResult{
		Payment: payment,
		Checkout: Checkout{
			Provider:     settings.Provider,
			DisplayName:  settings.DisplayName,
			AccountName:  settings.AccountName,
			AccountID:    settings.AccountID,
			KHQRImageURL: *settings.KHQRImageURL,
			Instructions: settings.Instructions,
		},
	}, nil
}

func (s Service) checkoutSLAIPayment(ctx context.Context, userID, packageID string) (CheckoutResult, error) {
	if s.slaiPaymentClient == nil {
		return CheckoutResult{}, ErrPaymentProviderDisabled
	}
	pkg, err := packages.NewRepository(s.pool).Get(ctx, packageID)
	if err != nil {
		return CheckoutResult{}, err
	}
	if !pkg.Active {
		return CheckoutResult{}, packages.ErrInvalidPackage
	}

	reference, err := s.reserveCheckoutReference(ctx, userID, pkg)
	if err != nil {
		return CheckoutResult{}, err
	}

	callbackURL := strings.TrimRight(s.cfg.SLAIPaymentCallbackBaseURL, "/") + "/v1/payments/slai-payment/callback"
	includeImage := true
	expiry := s.cfg.SLAIPaymentDefaultExpiry
	if expiry <= 0 {
		expiry = 30 * time.Minute
	}
	external, err := s.slaiPaymentClient.CreatePayment(ctx, slaipayment.CreatePaymentInput{
		Reference:      reference,
		MerchantPrefix: s.cfg.SLAIPaymentMerchantPrefix,
		Amount:         formatPaymentAmount(pkg.PriceMinor, pkg.Currency),
		Currency:       strings.ToUpper(pkg.Currency),
		CallbackURL:    callbackURL,
		ExpiresIn:      expiry.String(),
		IncludeQRImage: &includeImage,
		Metadata: map[string]any{
			"package_id": packageID,
			"user_id":    userID,
		},
	})
	if err != nil {
		_ = s.cancelCheckoutReference(ctx, reference, "payment provider create failed")
		return CheckoutResult{}, err
	}

	payment, err := s.attachExternalPayment(ctx, reference, external)
	if err != nil {
		return CheckoutResult{}, err
	}
	return CheckoutResult{
		Payment: payment,
		Checkout: Checkout{
			Provider:       ProviderBakongKHQR,
			DisplayName:    "Bakong KHQR",
			QRPayload:      payment.QRPayload,
			QRImageDataURI: payment.QRImageDataURI,
			Reference:      payment.CheckoutReference,
			ExpiresAt:      payment.ExpiresAt,
		},
	}, nil
}

func (s Service) reserveCheckoutReference(ctx context.Context, userID string, pkg packages.Package) (string, error) {
	for attempt := 0; attempt < 6; attempt++ {
		reference := randomCheckoutReference()
		_, err := s.pool.Exec(ctx, `
			INSERT INTO payments (
				user_id, package_id, provider, provider_ref, checkout_reference,
				amount_minor, currency, credit_units, status, provider_status
			)
			VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8, $9)
		`, userID, pkg.ID, ProviderBakongKHQR, reference, pkg.PriceMinor, strings.ToUpper(pkg.Currency), pkg.CreditUnits+pkg.BonusCreditUnits, StatusPendingPayment, "PENDING")
		if err == nil {
			return reference, nil
		}
		if isUniqueViolation(err) {
			continue
		}
		return "", err
	}
	return "", errors.New("could not allocate checkout reference")
}

func (s Service) cancelCheckoutReference(ctx context.Context, reference string, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payments
		SET status = $2, note = COALESCE(note, $3)
		WHERE provider = $1 AND checkout_reference = $4 AND status = $5
	`, ProviderBakongKHQR, StatusCancelled, reason, reference, StatusPendingPayment)
	return err
}

func (s Service) attachExternalPayment(ctx context.Context, reference string, external slaipayment.Payment) (Payment, error) {
	metadata, err := json.Marshal(external)
	if err != nil {
		return Payment{}, err
	}
	payment, err := scanPaymentDetail(s.pool.QueryRow(ctx, `
		UPDATE payments
		SET external_payment_id = $2,
		    provider_status = $3,
		    qr_payload = $4,
		    qr_image_data_uri = $5,
		    qr_md5 = $6,
		    expires_at = $7,
		    provider_metadata = $8,
		    status = $9
		WHERE provider = $1 AND checkout_reference = $10
		RETURNING id::text, user_id::text, package_id::text,
		          (SELECT cp.name FROM credit_packages cp WHERE cp.id = payments.package_id),
		          provider, provider_ref, amount_minor, currency, credit_units, status, admin_id::text, note,
		          EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = payments.id),
		          rejection_reason, CASE WHEN status = 'paid' THEN admin_payment_reference ELSE NULL END,
		          reviewed_by_admin_id::text, reviewed_at, created_at, updated_at, paid_at,
		          external_payment_id, checkout_reference, qr_payload, qr_image_data_uri, qr_md5,
		          expires_at, callback_received_at, provider_status, provider_transaction_id, provider_apv
	`, ProviderBakongKHQR, external.ID, external.Status, external.QRPayload, nilIfEmpty(external.QRImageDataURI), nilIfEmpty(external.QRMD5), nilIfZeroTime(external.ExpiresAt), string(metadata), mapExternalStatus(external.Status), reference))
	if err != nil {
		return Payment{}, mapPaymentScanErr(err)
	}
	return payment, nil
}

func (s Service) RefreshForUser(ctx context.Context, userID, paymentID string) (Payment, error) {
	payment, err := s.getPayment(ctx, paymentID)
	if err != nil {
		return Payment{}, err
	}
	if payment.UserID != userID {
		return Payment{}, ErrPaymentForbidden
	}
	if !s.cfg.SLAIPaymentEnabled || s.slaiPaymentClient == nil || payment.ExternalPaymentID == nil || *payment.ExternalPaymentID == "" {
		return payment, nil
	}
	external, err := s.slaiPaymentClient.GetPayment(ctx, *payment.ExternalPaymentID)
	if err != nil {
		return Payment{}, err
	}
	result, err := s.ApplySLAIPayment(ctx, external, false)
	if err != nil {
		return Payment{}, err
	}
	return result.Payment, nil
}

type ProviderPaymentResult struct {
	Payment Payment         `json:"payment"`
	Ledger  *ledger.Entry   `json:"ledger,omitempty"`
	Balance *ledger.Balance `json:"balance,omitempty"`
}

func (s Service) ApplySLAIPayment(ctx context.Context, external slaipayment.Payment, fromCallback bool) (ProviderPaymentResult, error) {
	if external.ID == "" || external.Reference == "" {
		return ProviderPaymentResult{}, ErrPaymentCallbackInvalid
	}
	var result ProviderPaymentResult
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		payment, err := scanPaymentDetail(tx.QueryRow(ctx, `
			SELECT p.id::text, p.user_id::text, p.package_id::text, cp.name, p.provider, p.provider_ref,
			       p.amount_minor, p.currency, p.credit_units, p.status, p.admin_id::text, p.note,
			       EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = p.id),
			       p.rejection_reason, CASE WHEN p.status = 'paid' THEN p.admin_payment_reference ELSE NULL END,
			       p.reviewed_by_admin_id::text, p.reviewed_at, p.created_at, p.updated_at, p.paid_at,
			       p.external_payment_id, p.checkout_reference, p.qr_payload, p.qr_image_data_uri, p.qr_md5,
			       p.expires_at, p.callback_received_at, p.provider_status, p.provider_transaction_id, p.provider_apv
			FROM payments p
			LEFT JOIN credit_packages cp ON cp.id = p.package_id
			WHERE p.provider = $1
			  AND (p.external_payment_id = $2 OR p.checkout_reference = $3 OR p.provider_ref = $3)
			ORDER BY CASE WHEN p.external_payment_id = $2 THEN 0 ELSE 1 END
			LIMIT 1
			FOR UPDATE OF p
		`, ProviderBakongKHQR, external.ID, external.Reference))
		if err != nil {
			return mapPaymentScanErr(err)
		}

		updated, ledgerEntry, balance, err := applyExternalPaymentInTx(ctx, tx, payment, external, fromCallback)
		if err != nil {
			return err
		}
		result.Payment = updated
		result.Ledger = ledgerEntry
		result.Balance = balance
		return nil
	})
	if err != nil {
		return ProviderPaymentResult{}, err
	}
	if result.Ledger != nil && result.Balance != nil {
		s.autoResumeAfterCredit(ctx, result.Payment.UserID, *result.Ledger, *result.Balance)
	}
	return result, nil
}

func (s Service) UploadProof(ctx context.Context, userID, paymentID string, input ProofUploadInput) (Payment, error) {
	if input.File.Path == "" || input.File.MIME == "" || input.File.Size <= 0 || input.File.SHA256 == "" {
		return Payment{}, ErrInvalidPaymentProof
	}
	input.TransactionRef = cleanOptional(input.TransactionRef)
	input.Note = cleanOptional(input.Note)

	var payment Payment
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		locked, err := scanPayment(tx.QueryRow(ctx, `
			SELECT p.id::text, p.user_id::text, p.package_id::text, cp.name, p.provider, p.provider_ref,
			       p.amount_minor, p.currency, p.credit_units, p.status, p.admin_id::text, p.note,
			       EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = p.id),
			       p.rejection_reason, CASE WHEN p.status = 'paid' THEN p.admin_payment_reference ELSE NULL END,
			       p.reviewed_by_admin_id::text, p.reviewed_at, p.created_at, p.updated_at, p.paid_at
			FROM payments p
			LEFT JOIN credit_packages cp ON cp.id = p.package_id
			WHERE p.id = $1
			FOR UPDATE OF p
		`, paymentID))
		if err != nil {
			return mapPaymentScanErr(err)
		}
		if locked.UserID != userID {
			return ErrPaymentForbidden
		}
		if locked.Status != StatusPendingProof && locked.Status != StatusRejected {
			return ErrInvalidPaymentState
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO payment_proofs (
				payment_id, user_id, file_path, file_name, file_mime, file_size,
				file_sha256, user_transaction_ref, user_note
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, paymentID, userID, input.File.Path, input.File.Name, input.File.MIME, input.File.Size, input.File.SHA256, input.TransactionRef, input.Note)
		if err != nil {
			return fmt.Errorf("insert payment proof: %w", err)
		}

		updated, err := scanPayment(tx.QueryRow(ctx, `
			UPDATE payments
			SET status = $2,
			    rejection_reason = NULL,
			    reviewed_by_admin_id = NULL,
			    reviewed_at = NULL
			WHERE id = $1
			RETURNING id::text, user_id::text, package_id::text, $3::text, provider, provider_ref,
			          amount_minor, currency, credit_units, status, admin_id::text, note,
			          true, rejection_reason, CASE WHEN status = 'paid' THEN admin_payment_reference ELSE NULL END,
			          reviewed_by_admin_id::text, reviewed_at, created_at, updated_at, paid_at
		`, paymentID, StatusPendingReview, locked.PackageName))
		if err != nil {
			return err
		}
		payment = updated
		return nil
	})
	if err != nil {
		return Payment{}, err
	}
	return payment, nil
}

func (s Service) LatestProofForUser(ctx context.Context, userID, paymentID string) (PaymentProof, error) {
	payment, err := s.GetForUser(ctx, userID, paymentID)
	if err != nil {
		return PaymentProof{}, err
	}
	return s.latestProof(ctx, payment.ID)
}

func (s Service) LatestProofForAdmin(ctx context.Context, paymentID string) (PaymentProof, error) {
	return s.latestProof(ctx, paymentID)
}

func (s Service) ListAdmin(ctx context.Context, filter AdminPaymentFilter) (AdminPaymentListResult, error) {
	filter = normalizeAdminPaymentFilter(filter)
	where, args := adminPaymentWhere(filter)
	countQuery := `
		SELECT count(*)
		FROM payments p
		JOIN users u ON u.id = p.user_id
		LEFT JOIN credit_packages cp ON cp.id = p.package_id
	` + where
	var total int64
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return AdminPaymentListResult{}, fmt.Errorf("count admin payments: %w", err)
	}

	args = append(args, filter.Limit, filter.Offset)
	query := adminPaymentSelect() + where + `
		ORDER BY CASE
			WHEN p.status = 'pending_payment' THEN 0
			WHEN p.status = 'needs_review' THEN 1
			WHEN p.status = 'pending_review' THEN 2
			ELSE 3
		END, p.created_at DESC, p.id DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return AdminPaymentListResult{}, fmt.Errorf("list admin payments: %w", err)
	}
	defer rows.Close()

	items := []AdminPaymentItem{}
	for rows.Next() {
		item, err := scanAdminPaymentItem(rows)
		if err != nil {
			return AdminPaymentListResult{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return AdminPaymentListResult{}, fmt.Errorf("iterate admin payments: %w", err)
	}
	return AdminPaymentListResult{Items: items, Limit: filter.Limit, Offset: filter.Offset, Total: total}, nil
}

func (s Service) GetAdmin(ctx context.Context, paymentID string) (AdminPaymentDetail, error) {
	item, err := scanAdminPaymentItem(s.pool.QueryRow(ctx, adminPaymentSelect()+` WHERE p.id = $1`, paymentID))
	if err != nil {
		return AdminPaymentDetail{}, mapPaymentScanErr(err)
	}
	detail := AdminPaymentDetail{AdminPaymentItem: item}
	proof, err := s.latestProof(ctx, paymentID)
	if err == nil {
		detail.Proof = &proof
	} else if !errors.Is(err, ErrInvalidPaymentProof) {
		return AdminPaymentDetail{}, err
	}
	return detail, nil
}

type ApproveResult struct {
	Payment Payment        `json:"payment"`
	Ledger  ledger.Entry   `json:"ledger"`
	Balance ledger.Balance `json:"balance"`
}

func (s Service) Approve(ctx context.Context, adminID, paymentID string, input ApproveInput) (ApproveResult, error) {
	normalized := NormalizePaymentReference(input.PaymentReference)
	if normalized == "" {
		return ApproveResult{}, ErrPaymentReferenceRequired
	}
	note := cleanOptional(input.Note)

	var result ApproveResult
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		payment, err := scanPayment(tx.QueryRow(ctx, `
			SELECT p.id::text, p.user_id::text, p.package_id::text, cp.name, p.provider, p.provider_ref,
			       p.amount_minor, p.currency, p.credit_units, p.status, p.admin_id::text, p.note,
			       EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = p.id),
			       p.rejection_reason, CASE WHEN p.status = 'paid' THEN p.admin_payment_reference ELSE NULL END,
			       p.reviewed_by_admin_id::text, p.reviewed_at, p.created_at, p.updated_at, p.paid_at
			FROM payments p
			LEFT JOIN credit_packages cp ON cp.id = p.package_id
			WHERE p.id = $1
			FOR UPDATE OF p
		`, paymentID))
		if err != nil {
			return mapPaymentScanErr(err)
		}
		if payment.Status == StatusPaid {
			return ErrPaymentAlreadyPaid
		}
		if payment.Status != StatusPendingReview && payment.Status != StatusNeedsReview {
			return ErrInvalidPaymentState
		}

		var duplicateID string
		err = tx.QueryRow(ctx, `
			SELECT id::text
			FROM payments
			WHERE provider = $1
			  AND admin_payment_reference_normalized = $2
			  AND status = 'paid'
			  AND id <> $3
			LIMIT 1
		`, payment.Provider, normalized, payment.ID).Scan(&duplicateID)
		if err == nil {
			return ErrDuplicatePaymentReference
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		updated, err := scanPayment(tx.QueryRow(ctx, `
			UPDATE payments
			SET status = $2,
			    paid_at = COALESCE(paid_at, now()),
			    reviewed_by_admin_id = $3,
			    reviewed_at = now(),
			    admin_payment_reference = $4,
			    admin_payment_reference_normalized = $5,
			    admin_id = COALESCE(admin_id, $3),
			    note = COALESCE($6, note)
			WHERE id = $1
			RETURNING id::text, user_id::text, package_id::text, $7::text, provider, provider_ref,
			          amount_minor, currency, credit_units, status, admin_id::text, note,
			          EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = payments.id),
			          rejection_reason, CASE WHEN status = 'paid' THEN admin_payment_reference ELSE NULL END,
			          reviewed_by_admin_id::text, reviewed_at, created_at, updated_at, paid_at
		`, payment.ID, StatusPaid, adminID, strings.TrimSpace(input.PaymentReference), normalized, note, payment.PackageName))
		if err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicatePaymentReference
			}
			return err
		}

		paymentID := updated.ID
		idempotency := "payment_approve:" + updated.ID
		reason := "Bakong KHQR manual payment approved"
		entry, balance, err := ledger.NewService(tx).Mutate(ctx, ledger.Mutation{
			UserID:         updated.UserID,
			Type:           ledger.TypePaymentCredit,
			Source:         "payment_provider",
			DeltaUnits:     updated.CreditUnits,
			PaymentID:      &paymentID,
			AdminID:        &adminID,
			IdempotencyKey: &idempotency,
			Reason:         &reason,
			Metadata: map[string]any{
				"provider":                     updated.Provider,
				"adminPaymentReference":        strings.TrimSpace(input.PaymentReference),
				"adminPaymentReferenceHashKey": normalized,
				"amountMinor":                  updated.AmountMinor,
				"currency":                     updated.Currency,
			},
		})
		if err != nil {
			return err
		}

		targetType := "payment"
		targetID := updated.ID
		if err := admin.NewAuditLogger(tx).Log(ctx, adminID, "payment_approved", &targetType, &targetID, map[string]any{
			"provider":            updated.Provider,
			"userId":              updated.UserID,
			"creditUnits":         updated.CreditUnits,
			"amountMinor":         updated.AmountMinor,
			"currency":            updated.Currency,
			"paymentReference":    strings.TrimSpace(input.PaymentReference),
			"normalizedReference": normalized,
		}); err != nil {
			return err
		}

		result = ApproveResult{Payment: updated, Ledger: entry, Balance: balance}
		return nil
	})
	if err != nil {
		return ApproveResult{}, err
	}
	s.autoResumeAfterCredit(ctx, result.Payment.UserID, result.Ledger, result.Balance)
	return result, nil
}

func (s Service) Reject(ctx context.Context, adminID, paymentID string, input RejectInput) (Payment, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return Payment{}, ErrInvalidPaymentState
	}

	var payment Payment
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		locked, err := scanPayment(tx.QueryRow(ctx, `
			SELECT p.id::text, p.user_id::text, p.package_id::text, cp.name, p.provider, p.provider_ref,
			       p.amount_minor, p.currency, p.credit_units, p.status, p.admin_id::text, p.note,
			       EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = p.id),
			       p.rejection_reason, CASE WHEN p.status = 'paid' THEN p.admin_payment_reference ELSE NULL END,
			       p.reviewed_by_admin_id::text, p.reviewed_at, p.created_at, p.updated_at, p.paid_at
			FROM payments p
			LEFT JOIN credit_packages cp ON cp.id = p.package_id
			WHERE p.id = $1
			FOR UPDATE OF p
		`, paymentID))
		if err != nil {
			return mapPaymentScanErr(err)
		}
		if locked.Status == StatusPaid {
			return ErrPaymentAlreadyPaid
		}
		if locked.Status != StatusPendingReview && locked.Status != StatusPendingProof && locked.Status != StatusNeedsReview {
			return ErrInvalidPaymentState
		}

		updated, err := scanPayment(tx.QueryRow(ctx, `
			UPDATE payments
			SET status = $2,
			    reviewed_by_admin_id = $3,
			    reviewed_at = now(),
			    rejection_reason = $4
			WHERE id = $1
			RETURNING id::text, user_id::text, package_id::text, $5::text, provider, provider_ref,
			          amount_minor, currency, credit_units, status, admin_id::text, note,
			          EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = payments.id),
			          rejection_reason, CASE WHEN status = 'paid' THEN admin_payment_reference ELSE NULL END,
			          reviewed_by_admin_id::text, reviewed_at, created_at, updated_at, paid_at
		`, locked.ID, StatusRejected, adminID, reason, locked.PackageName))
		if err != nil {
			return err
		}

		targetType := "payment"
		targetID := updated.ID
		if err := admin.NewAuditLogger(tx).Log(ctx, adminID, "payment_rejected", &targetType, &targetID, map[string]any{
			"provider": updated.Provider,
			"userId":   updated.UserID,
			"reason":   reason,
		}); err != nil {
			return err
		}
		payment = updated
		return nil
	})
	if err != nil {
		return Payment{}, err
	}
	return payment, nil
}

// NormalizePaymentReference trims leading/trailing space, uppercases letters,
// removes all internal Unicode whitespace, and keeps only printable ASCII
// characters. This makes duplicate checks robust against spacing/case changes
// without trusting the user-submitted reference.
func NormalizePaymentReference(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsSpace(r) {
			continue
		}
		if r >= 33 && r <= 126 {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

type ManualTopUpResult struct {
	Payment Payment        `json:"payment"`
	Ledger  ledger.Entry   `json:"ledger"`
	Balance ledger.Balance `json:"balance"`
}

func (s Service) ManualTopUp(ctx context.Context, adminID string, input ManualTopUpInput) (ManualTopUpResult, error) {
	if strings.TrimSpace(input.UserID) == "" || input.AmountMinor < 0 || input.CreditUnits <= 0 {
		return ManualTopUpResult{}, ErrInvalidTopUp
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}

	var result ManualTopUpResult
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if input.IdempotencyKey != nil && *input.IdempotencyKey != "" {
			payment, found, err := findPaymentByProviderRef(ctx, tx, *input.IdempotencyKey)
			if err != nil {
				return err
			}
			if found {
				if payment.UserID != input.UserID || payment.AmountMinor != input.AmountMinor || payment.CreditUnits != input.CreditUnits || payment.Currency != strings.ToUpper(input.Currency) {
					return ErrIdempotencyConflict
				}
				entry, balance, err := findTopUpResult(ctx, tx, payment)
				if err != nil {
					return err
				}
				result = ManualTopUpResult{Payment: payment, Ledger: entry, Balance: balance}
				return nil
			}
		}

		payment, err := insertManualPayment(ctx, tx, adminID, input)
		if err != nil {
			return err
		}

		paymentID := payment.ID
		reason := "manual admin top-up"
		mutation := ledger.Mutation{
			UserID:         input.UserID,
			Type:           ledger.TypePaymentCredit,
			Source:         ledger.SourceAdmin,
			DeltaUnits:     input.CreditUnits,
			PaymentID:      &paymentID,
			AdminID:        &adminID,
			IdempotencyKey: input.IdempotencyKey,
			Reason:         &reason,
			Metadata: map[string]any{
				"amountMinor": input.AmountMinor,
				"currency":    strings.ToUpper(input.Currency),
			},
		}
		entry, balance, err := ledger.NewService(tx).Mutate(ctx, mutation)
		if err != nil {
			return err
		}

		targetType := "payment"
		targetID := payment.ID
		if err := admin.NewAuditLogger(tx).Log(ctx, adminID, "manual_topup_created", &targetType, &targetID, map[string]any{
			"userId":      input.UserID,
			"creditUnits": input.CreditUnits,
			"amountMinor": input.AmountMinor,
			"currency":    strings.ToUpper(input.Currency),
		}); err != nil {
			return err
		}

		result = ManualTopUpResult{Payment: payment, Ledger: entry, Balance: balance}
		return nil
	})
	if err != nil {
		return ManualTopUpResult{}, err
	}
	s.autoResumeAfterCredit(ctx, result.Payment.UserID, result.Ledger, result.Balance)

	return result, nil
}

func insertManualPayment(ctx context.Context, tx pgx.Tx, adminID string, input ManualTopUpInput) (Payment, error) {
	payment, err := scanPayment(tx.QueryRow(ctx, `
		INSERT INTO payments (
			user_id, package_id, provider, provider_ref, amount_minor, currency,
			credit_units, status, admin_id, note, paid_at, reviewed_by_admin_id, reviewed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), $9, now())
		RETURNING id::text, user_id::text, package_id::text, NULL::text, provider, provider_ref, amount_minor, currency,
		          credit_units, status, admin_id::text, note, false, rejection_reason,
		          CASE WHEN status = 'paid' THEN admin_payment_reference ELSE NULL END,
		          reviewed_by_admin_id::text, reviewed_at, created_at, updated_at, paid_at
	`, input.UserID, input.PackageID, ProviderManual, input.IdempotencyKey, input.AmountMinor, strings.ToUpper(input.Currency), input.CreditUnits, StatusPaid, adminID, input.Note))
	if err != nil {
		return Payment{}, fmt.Errorf("insert manual payment: %w", err)
	}
	return payment, nil
}

func findPaymentByProviderRef(ctx context.Context, tx pgx.Tx, providerRef string) (Payment, bool, error) {
	payment, err := scanPayment(tx.QueryRow(ctx, `
		SELECT p.id::text, p.user_id::text, p.package_id::text, cp.name, p.provider, p.provider_ref,
		       p.amount_minor, p.currency, p.credit_units, p.status, p.admin_id::text, p.note,
		       EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = p.id),
		       p.rejection_reason, CASE WHEN p.status = 'paid' THEN p.admin_payment_reference ELSE NULL END,
		       p.reviewed_by_admin_id::text, p.reviewed_at, p.created_at, p.updated_at, p.paid_at
		FROM payments p
		LEFT JOIN credit_packages cp ON cp.id = p.package_id
		WHERE p.provider = $1 AND p.provider_ref = $2
	`, ProviderManual, providerRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, false, nil
	}
	if err != nil {
		return Payment{}, false, fmt.Errorf("find manual payment by provider ref: %w", err)
	}
	return payment, true, nil
}

func findTopUpResult(ctx context.Context, tx pgx.Tx, payment Payment) (ledger.Entry, ledger.Balance, error) {
	var entry ledger.Entry
	var metadataBytes []byte
	err := tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, type, source, delta_units, balance_after_units,
		       payment_id::text, usage_event_id::text, admin_id::text, idempotency_key, reason, metadata, created_at
		FROM credit_ledger_entries
		WHERE payment_id = $1
	`, payment.ID).Scan(
		&entry.ID,
		&entry.UserID,
		&entry.Type,
		&entry.Source,
		&entry.DeltaUnits,
		&entry.BalanceAfterUnits,
		&entry.PaymentID,
		&entry.UsageEventID,
		&entry.AdminID,
		&entry.IdempotencyKey,
		&entry.Reason,
		&metadataBytes,
		&entry.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Entry{}, ledger.Balance{}, errors.New("idempotent manual top-up missing ledger entry")
	}
	if err != nil {
		return ledger.Entry{}, ledger.Balance{}, err
	}
	if len(metadataBytes) > 0 {
		entry.Metadata = map[string]any{}
		_ = json.Unmarshal(metadataBytes, &entry.Metadata)
	}
	balance, err := ledger.NewService(tx).GetBalance(ctx, payment.UserID)
	if err != nil {
		return ledger.Entry{}, ledger.Balance{}, err
	}
	return entry, balance, nil
}

func (s Service) getPayment(ctx context.Context, paymentID string) (Payment, error) {
	payment, err := scanPaymentDetail(s.pool.QueryRow(ctx, `
		SELECT p.id::text, p.user_id::text, p.package_id::text, cp.name, p.provider, p.provider_ref,
		       p.amount_minor, p.currency, p.credit_units, p.status, p.admin_id::text, p.note,
		       EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = p.id),
		       p.rejection_reason, CASE WHEN p.status = 'paid' THEN p.admin_payment_reference ELSE NULL END,
		       p.reviewed_by_admin_id::text, p.reviewed_at, p.created_at, p.updated_at, p.paid_at,
		       p.external_payment_id, p.checkout_reference, p.qr_payload, p.qr_image_data_uri, p.qr_md5,
		       p.expires_at, p.callback_received_at, p.provider_status, p.provider_transaction_id, p.provider_apv
		FROM payments p
		LEFT JOIN credit_packages cp ON cp.id = p.package_id
		WHERE p.id = $1
	`, paymentID))
	if err != nil {
		return Payment{}, mapPaymentScanErr(err)
	}
	return payment, nil
}

func (s Service) latestProof(ctx context.Context, paymentID string) (PaymentProof, error) {
	var proof PaymentProof
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, payment_id::text, user_id::text, file_path, file_name, file_mime,
		       file_size, file_sha256, user_transaction_ref, user_note, uploaded_at
		FROM payment_proofs
		WHERE payment_id = $1
		ORDER BY uploaded_at DESC, id DESC
		LIMIT 1
	`, paymentID).Scan(
		&proof.ID,
		&proof.PaymentID,
		&proof.UserID,
		&proof.FilePath,
		&proof.FileName,
		&proof.FileMIME,
		&proof.FileSize,
		&proof.FileSHA256,
		&proof.UserTransactionRef,
		&proof.UserNote,
		&proof.UploadedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentProof{}, ErrInvalidPaymentProof
	}
	if err != nil {
		return PaymentProof{}, fmt.Errorf("get payment proof: %w", err)
	}
	return proof, nil
}

func applyExternalPaymentInTx(ctx context.Context, tx pgx.Tx, payment Payment, external slaipayment.Payment, fromCallback bool) (Payment, *ledger.Entry, *ledger.Balance, error) {
	if payment.Provider != ProviderBakongKHQR {
		return Payment{}, nil, nil, ErrPaymentProviderMismatch
	}
	if payment.ExternalPaymentID != nil && *payment.ExternalPaymentID != "" && *payment.ExternalPaymentID != external.ID {
		updated, err := updatePaymentExternalState(ctx, tx, payment.ID, StatusNeedsReview, external, fromCallback, nil)
		return updated, nil, nil, err
	}
	amountMinor, err := parsePaymentAmountMinor(external.Amount, external.Currency)
	if err != nil || amountMinor != payment.AmountMinor || !strings.EqualFold(external.Currency, payment.Currency) {
		updated, updateErr := updatePaymentExternalState(ctx, tx, payment.ID, StatusNeedsReview, external, fromCallback, nil)
		return updated, nil, nil, updateErr
	}

	localStatus := mapExternalStatus(external.Status)
	if payment.Status == StatusPaid && localStatus != StatusPaid {
		return payment, nil, nil, nil
	}
	if localStatus != StatusPaid {
		updated, err := updatePaymentExternalState(ctx, tx, payment.ID, localStatus, external, fromCallback, nil)
		return updated, nil, nil, err
	}

	txID, _ := externalPaymentTransactionRefs(external)
	if txID != nil {
		var duplicateID string
		err := tx.QueryRow(ctx, `
			SELECT id::text
			FROM payments
			WHERE provider = $1
			  AND provider_transaction_id = $2
			  AND status = 'paid'
			  AND id <> $3
			LIMIT 1
		`, ProviderBakongKHQR, *txID, payment.ID).Scan(&duplicateID)
		if err == nil {
			updated, updateErr := updatePaymentExternalState(ctx, tx, payment.ID, StatusNeedsReview, external, fromCallback, nil)
			return updated, nil, nil, updateErr
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Payment{}, nil, nil, err
		}
	}

	paidAt := external.PaidAt
	if paidAt == nil {
		now := time.Now().UTC()
		paidAt = &now
	}
	updated, err := updatePaymentExternalState(ctx, tx, payment.ID, StatusPaid, external, fromCallback, paidAt)
	if err != nil {
		if isUniqueViolation(err) {
			needsReview, updateErr := updatePaymentExternalState(ctx, tx, payment.ID, StatusNeedsReview, external, fromCallback, nil)
			return needsReview, nil, nil, updateErr
		}
		return Payment{}, nil, nil, err
	}
	if payment.Status == StatusPaid {
		return updated, nil, nil, nil
	}

	paymentID := updated.ID
	idempotency := "slai_payment_paid:" + updated.ID
	reason := "Bakong KHQR payment received"
	entry, balance, err := ledger.NewService(tx).Mutate(ctx, ledger.Mutation{
		UserID:         updated.UserID,
		Type:           ledger.TypePaymentCredit,
		Source:         "payment_provider",
		DeltaUnits:     updated.CreditUnits,
		PaymentID:      &paymentID,
		IdempotencyKey: &idempotency,
		Reason:         &reason,
		Metadata: map[string]any{
			"provider":              updated.Provider,
			"externalPaymentId":     external.ID,
			"checkoutReference":     external.Reference,
			"providerStatus":        external.Status,
			"providerTransactionId": stringValue(txID),
			"amountMinor":           updated.AmountMinor,
			"currency":              updated.Currency,
		},
	})
	if err != nil {
		return Payment{}, nil, nil, err
	}
	return updated, &entry, &balance, nil
}

func updatePaymentExternalState(ctx context.Context, tx pgx.Tx, paymentID string, status string, external slaipayment.Payment, fromCallback bool, paidAt *time.Time) (Payment, error) {
	metadata, err := json.Marshal(external)
	if err != nil {
		return Payment{}, err
	}
	var callbackAt *time.Time
	if fromCallback {
		now := time.Now().UTC()
		callbackAt = &now
	}
	txID, apv := externalPaymentTransactionRefs(external)
	expiresAt := nilIfZeroTime(external.ExpiresAt)
	payment, err := scanPaymentDetail(tx.QueryRow(ctx, `
		UPDATE payments
		SET status = $2,
		    external_payment_id = COALESCE(NULLIF($3, ''), external_payment_id),
		    provider_status = $4,
		    provider_metadata = $5::jsonb,
		    callback_received_at = COALESCE($6, callback_received_at),
		    provider_transaction_id = COALESCE($7, provider_transaction_id),
		    provider_apv = COALESCE($8, provider_apv),
		    expires_at = COALESCE($9, expires_at),
		    paid_at = COALESCE($10, paid_at),
		    qr_payload = COALESCE($11, qr_payload),
		    qr_image_data_uri = COALESCE($12, qr_image_data_uri),
		    qr_md5 = COALESCE($13, qr_md5)
		WHERE id = $1
		RETURNING id::text, user_id::text, package_id::text,
		          (SELECT cp.name FROM credit_packages cp WHERE cp.id = payments.package_id),
		          provider, provider_ref, amount_minor, currency, credit_units, status, admin_id::text, note,
		          EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.payment_id = payments.id),
		          rejection_reason, CASE WHEN status = 'paid' THEN admin_payment_reference ELSE NULL END,
		          reviewed_by_admin_id::text, reviewed_at, created_at, updated_at, paid_at,
		          external_payment_id, checkout_reference, qr_payload, qr_image_data_uri, qr_md5,
		          expires_at, callback_received_at, provider_status, provider_transaction_id, provider_apv
	`, paymentID, status, external.ID, external.Status, string(metadata), callbackAt, txID, apv, expiresAt, paidAt, nilIfEmpty(external.QRPayload), nilIfEmpty(external.QRImageDataURI), nilIfEmpty(external.QRMD5)))
	if err != nil {
		return Payment{}, err
	}
	return payment, nil
}

func externalPaymentTransactionRefs(external slaipayment.Payment) (*string, *string) {
	if external.Telegram == nil {
		return nil, nil
	}
	return nilIfEmpty(external.Telegram.TransactionID), nilIfEmpty(external.Telegram.APV)
}

func nilIfZeroTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func scanPaymentDetail(row pgx.Row) (Payment, error) {
	var payment Payment
	err := row.Scan(
		&payment.ID,
		&payment.UserID,
		&payment.PackageID,
		&payment.PackageName,
		&payment.Provider,
		&payment.ProviderRef,
		&payment.AmountMinor,
		&payment.Currency,
		&payment.CreditUnits,
		&payment.Status,
		&payment.AdminID,
		&payment.Note,
		&payment.ProofUploaded,
		&payment.RejectionReason,
		&payment.AdminPaymentReference,
		&payment.ReviewedByAdminID,
		&payment.ReviewedAt,
		&payment.CreatedAt,
		&payment.UpdatedAt,
		&payment.PaidAt,
		&payment.ExternalPaymentID,
		&payment.CheckoutReference,
		&payment.QRPayload,
		&payment.QRImageDataURI,
		&payment.QRMD5,
		&payment.ExpiresAt,
		&payment.CallbackReceivedAt,
		&payment.ProviderStatus,
		&payment.ProviderTransactionID,
		&payment.ProviderAPV,
	)
	if err != nil {
		return Payment{}, err
	}
	return payment, nil
}

func scanPayment(row pgx.Row) (Payment, error) {
	var payment Payment
	err := row.Scan(
		&payment.ID,
		&payment.UserID,
		&payment.PackageID,
		&payment.PackageName,
		&payment.Provider,
		&payment.ProviderRef,
		&payment.AmountMinor,
		&payment.Currency,
		&payment.CreditUnits,
		&payment.Status,
		&payment.AdminID,
		&payment.Note,
		&payment.ProofUploaded,
		&payment.RejectionReason,
		&payment.AdminPaymentReference,
		&payment.ReviewedByAdminID,
		&payment.ReviewedAt,
		&payment.CreatedAt,
		&payment.UpdatedAt,
		&payment.PaidAt,
	)
	if err != nil {
		return Payment{}, err
	}
	return payment, nil
}

func scanPaymentSettings(row pgx.Row) (PaymentSettings, error) {
	var settings PaymentSettings
	err := row.Scan(
		&settings.ID,
		&settings.Provider,
		&settings.Enabled,
		&settings.DisplayName,
		&settings.AccountName,
		&settings.AccountID,
		&settings.KHQRImagePath,
		&settings.KHQRImageMIME,
		&settings.Instructions,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)
	return settings, err
}

func adminPaymentSelect() string {
	return `
		SELECT p.id::text, p.user_id::text, u.email, p.package_id::text, cp.name,
		       p.provider, p.provider_ref, p.external_payment_id, p.checkout_reference,
		       p.expires_at, p.provider_status, p.provider_transaction_id, p.provider_apv,
		       p.amount_minor, p.currency, p.credit_units,
		       p.status, p.admin_id::text, p.note, p.created_at, p.updated_at, p.paid_at,
		       latest_proof.file_sha256,
		       latest_proof.file_sha256 IS NOT NULL,
		       COALESCE(duplicate_counts.duplicate_count, 0),
		       p.reviewed_by_admin_id::text, reviewer.email, p.reviewed_at,
		       p.admin_payment_reference, p.rejection_reason
		FROM payments p
		JOIN users u ON u.id = p.user_id
		LEFT JOIN users reviewer ON reviewer.id = p.reviewed_by_admin_id
		LEFT JOIN credit_packages cp ON cp.id = p.package_id
		LEFT JOIN LATERAL (
			SELECT pp.file_sha256
			FROM payment_proofs pp
			WHERE pp.payment_id = p.id
			ORDER BY pp.uploaded_at DESC, pp.id DESC
			LIMIT 1
		) latest_proof ON true
		LEFT JOIN LATERAL (
			SELECT count(*) AS duplicate_count
			FROM payment_proofs pp2
			WHERE latest_proof.file_sha256 IS NOT NULL
			  AND pp2.file_sha256 = latest_proof.file_sha256
			  AND pp2.payment_id <> p.id
		) duplicate_counts ON true
	`
}

func scanAdminPaymentItem(row pgx.Row) (AdminPaymentItem, error) {
	var item AdminPaymentItem
	err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.UserEmail,
		&item.PackageID,
		&item.PackageName,
		&item.Provider,
		&item.ProviderRef,
		&item.ExternalPaymentID,
		&item.CheckoutReference,
		&item.ExpiresAt,
		&item.ProviderStatus,
		&item.ProviderTransactionID,
		&item.ProviderAPV,
		&item.AmountMinor,
		&item.Currency,
		&item.CreditUnits,
		&item.Status,
		&item.AdminID,
		&item.Note,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.PaidAt,
		&item.ProofFileSHA256,
		&item.ProofUploaded,
		&item.DuplicateProofCount,
		&item.ReviewedByAdminID,
		&item.ReviewedByAdminEmail,
		&item.ReviewedAt,
		&item.AdminPaymentReference,
		&item.RejectionReason,
	)
	if err != nil {
		return AdminPaymentItem{}, mapPaymentScanErr(err)
	}
	return item, nil
}

func normalizeAdminPaymentFilter(filter AdminPaymentFilter) AdminPaymentFilter {
	filter.Status = strings.TrimSpace(filter.Status)
	filter.UserID = strings.TrimSpace(filter.UserID)
	filter.Provider = strings.TrimSpace(filter.Provider)
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

func adminPaymentWhere(filter AdminPaymentFilter) (string, []any) {
	args := []any{}
	clauses := []string{}
	if filter.Status != "" {
		if filter.Status == "review_queue" {
			clauses = append(clauses, "p.status IN ('pending_review', 'needs_review')")
		} else {
			args = append(args, filter.Status)
			clauses = append(clauses, fmt.Sprintf("p.status = $%d", len(args)))
		}
	}
	if filter.UserID != "" {
		args = append(args, filter.UserID)
		clauses = append(clauses, fmt.Sprintf("p.user_id::text = $%d", len(args)))
	}
	if filter.Provider != "" {
		args = append(args, filter.Provider)
		clauses = append(clauses, fmt.Sprintf("p.provider = $%d", len(args)))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		clauses = append(clauses, fmt.Sprintf("p.created_at >= $%d", len(args)))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		clauses = append(clauses, fmt.Sprintf("p.created_at <= $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func mapExternalStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PAID":
		return StatusPaid
	case "EXPIRED":
		return StatusExpired
	case "NEEDS_REVIEW":
		return StatusNeedsReview
	case "PENDING", "":
		return StatusPendingPayment
	default:
		return StatusNeedsReview
	}
}

func formatPaymentAmount(minor int64, currency string) string {
	if strings.EqualFold(currency, "KHR") {
		return strconv.FormatInt(minor, 10)
	}
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}

func parsePaymentAmountMinor(value string, currency string) (int64, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	if cleaned == "" || strings.HasPrefix(cleaned, "-") {
		return 0, fmt.Errorf("invalid payment amount %q", value)
	}
	if strings.EqualFold(currency, "KHR") {
		if strings.Contains(cleaned, ".") {
			parts := strings.SplitN(cleaned, ".", 2)
			cleaned = parts[0]
		}
		return strconv.ParseInt(cleaned, 10, 64)
	}
	parts := strings.SplitN(cleaned, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 2 {
		fraction = fraction[:2]
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, err
	}
	return whole*100 + cents, nil
}

func randomCheckoutReference() string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return strings.ToUpper(strconv.FormatInt(time.Now().UnixNano()%0xffffff, 16))
	}
	return strings.ToUpper(hex.EncodeToString(buf))
}

func nilIfEmpty(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func mapPaymentScanErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPaymentNotFound
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func khqrImageURL(settings PaymentSettings) *string {
	if settings.KHQRImagePath == nil {
		return nil
	}
	url := "/v1/payment-settings/bakong-khqr/khqr-image"
	return &url
}
