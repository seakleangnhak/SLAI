package payments

import "time"

const (
	ProviderManual     = "manual"
	ProviderBakongKHQR = "bakong_khqr"

	StatusPendingPayment = "pending_payment"
	StatusPendingProof   = "pending_proof"
	StatusPendingReview  = "pending_review"
	StatusPaid           = "paid"
	StatusRejected       = "rejected"
	StatusCancelled      = "cancelled"
	StatusExpired        = "expired"
	StatusNeedsReview    = "needs_review"
)

type Payment struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"userId"`
	PackageID             *string    `json:"packageId,omitempty"`
	PackageName           *string    `json:"packageName,omitempty"`
	Provider              string     `json:"provider"`
	ProviderRef           *string    `json:"providerRef,omitempty"`
	ExternalPaymentID     *string    `json:"externalPaymentId,omitempty"`
	CheckoutReference     *string    `json:"checkoutReference,omitempty"`
	QRPayload             *string    `json:"qrPayload,omitempty"`
	QRImageDataURI        *string    `json:"qrImageDataUri,omitempty"`
	QRMD5                 *string    `json:"qrMd5,omitempty"`
	ExpiresAt             *time.Time `json:"expiresAt,omitempty"`
	CallbackReceivedAt    *time.Time `json:"callbackReceivedAt,omitempty"`
	ProviderStatus        *string    `json:"providerStatus,omitempty"`
	ProviderTransactionID *string    `json:"providerTransactionId,omitempty"`
	ProviderAPV           *string    `json:"providerApv,omitempty"`
	AmountMinor           int64      `json:"amountMinor"`
	Currency              string     `json:"currency"`
	CreditUnits           int64      `json:"creditUnits"`
	Status                string     `json:"status"`
	AdminID               *string    `json:"adminId,omitempty"`
	Note                  *string    `json:"note,omitempty"`
	ProofUploaded         bool       `json:"proofUploaded"`
	RejectionReason       *string    `json:"rejectionReason,omitempty"`
	AdminPaymentReference *string    `json:"adminPaymentReference,omitempty"`
	ReviewedByAdminID     *string    `json:"reviewedByAdminId,omitempty"`
	ReviewedAt            *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	PaidAt                *time.Time `json:"paidAt,omitempty"`
}

type ManualTopUpInput struct {
	UserID         string  `json:"userId"`
	PackageID      *string `json:"packageId"`
	AmountMinor    int64   `json:"amountMinor"`
	Currency       string  `json:"currency"`
	CreditUnits    int64   `json:"creditUnits"`
	Note           *string `json:"note"`
	IdempotencyKey *string `json:"idempotencyKey"`
}

type PaymentSettings struct {
	ID            string    `json:"id,omitempty"`
	Provider      string    `json:"provider"`
	Enabled       bool      `json:"enabled"`
	DisplayName   string    `json:"display_name"`
	AccountName   *string   `json:"account_name,omitempty"`
	AccountID     *string   `json:"account_id,omitempty"`
	KHQRImageURL  *string   `json:"khqr_image_url,omitempty"`
	KHQRImagePath *string   `json:"-"`
	KHQRImageMIME *string   `json:"-"`
	Instructions  *string   `json:"instructions,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PaymentSettingsInput struct {
	Enabled      bool    `json:"enabled"`
	DisplayName  string  `json:"display_name"`
	AccountName  *string `json:"account_name"`
	AccountID    *string `json:"account_id"`
	Instructions *string `json:"instructions"`
}

type PaymentProviderStatus struct {
	Provider                  string `json:"provider"`
	Mode                      string `json:"mode"`
	Enabled                   bool   `json:"enabled"`
	BaseURLConfigured         bool   `json:"base_url_configured"`
	APIKeyConfigured          bool   `json:"api_key_configured"`
	CallbackBaseURLConfigured bool   `json:"callback_base_url_configured"`
	CallbackSecretConfigured  bool   `json:"callback_secret_configured"`
	MerchantPrefix            string `json:"merchant_prefix,omitempty"`
	DefaultExpirySeconds      int64  `json:"default_expiry_seconds"`
}

type StoredFile struct {
	Path   string
	Name   string
	MIME   string
	Size   int64
	SHA256 string
}

type PaymentProof struct {
	ID                 string    `json:"id"`
	PaymentID          string    `json:"payment_id"`
	UserID             string    `json:"user_id"`
	FilePath           string    `json:"-"`
	FileName           string    `json:"file_name"`
	FileMIME           string    `json:"file_mime"`
	FileSize           int64     `json:"file_size"`
	FileSHA256         string    `json:"file_sha256"`
	UserTransactionRef *string   `json:"user_transaction_ref,omitempty"`
	UserNote           *string   `json:"user_note,omitempty"`
	UploadedAt         time.Time `json:"uploaded_at"`
}

type Checkout struct {
	Provider       string     `json:"provider"`
	DisplayName    string     `json:"display_name"`
	AccountName    *string    `json:"account_name,omitempty"`
	AccountID      *string    `json:"account_id,omitempty"`
	KHQRImageURL   string     `json:"khqr_image_url,omitempty"`
	QRPayload      *string    `json:"qr_payload,omitempty"`
	QRImageDataURI *string    `json:"qr_image_data_uri,omitempty"`
	Reference      *string    `json:"reference,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Instructions   *string    `json:"instructions,omitempty"`
}

type CheckoutResult struct {
	Payment  Payment  `json:"payment"`
	Checkout Checkout `json:"checkout"`
}

type ProofUploadInput struct {
	TransactionRef *string
	Note           *string
	File           StoredFile
}

type AdminPaymentFilter struct {
	Status   string
	UserID   string
	Provider string
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

type AdminPaymentItem struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"user_id"`
	UserEmail             string     `json:"user_email"`
	PackageID             *string    `json:"package_id,omitempty"`
	PackageName           *string    `json:"package_name,omitempty"`
	Provider              string     `json:"provider"`
	ProviderRef           *string    `json:"provider_ref,omitempty"`
	ExternalPaymentID     *string    `json:"external_payment_id,omitempty"`
	CheckoutReference     *string    `json:"checkout_reference,omitempty"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	ProviderStatus        *string    `json:"provider_status,omitempty"`
	ProviderTransactionID *string    `json:"provider_transaction_id,omitempty"`
	ProviderAPV           *string    `json:"provider_apv,omitempty"`
	AmountMinor           int64      `json:"amount_minor"`
	Currency              string     `json:"currency"`
	CreditUnits           int64      `json:"credit_units"`
	Status                string     `json:"status"`
	AdminID               *string    `json:"admin_id,omitempty"`
	Note                  *string    `json:"note,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	PaidAt                *time.Time `json:"paid_at,omitempty"`
	ProofUploaded         bool       `json:"proof_uploaded"`
	ProofFileSHA256       *string    `json:"proof_file_sha256,omitempty"`
	DuplicateProofCount   int64      `json:"duplicate_proof_count"`
	ReviewedByAdminID     *string    `json:"reviewed_by_admin_id,omitempty"`
	ReviewedByAdminEmail  *string    `json:"reviewed_by_admin_email,omitempty"`
	ReviewedAt            *time.Time `json:"reviewed_at,omitempty"`
	AdminPaymentReference *string    `json:"admin_payment_reference,omitempty"`
	RejectionReason       *string    `json:"rejection_reason,omitempty"`
}

type AdminPaymentListResult struct {
	Items  []AdminPaymentItem `json:"items"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
	Total  int64              `json:"total"`
}

type AdminPaymentDetail struct {
	AdminPaymentItem
	Proof *PaymentProof `json:"proof,omitempty"`
}

type ApproveInput struct {
	PaymentReference string  `json:"payment_reference"`
	Note             *string `json:"note"`
}

type RejectInput struct {
	Reason string `json:"reason"`
}

type ReviewResult struct {
	Payment Payment `json:"payment"`
	Ledger  any     `json:"ledger,omitempty"`
	Balance any     `json:"balance,omitempty"`
}
