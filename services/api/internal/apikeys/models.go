package apikeys

import "time"

const (
	StatusActive    = "ACTIVE"
	StatusSuspended = "SUSPENDED"
	StatusRevoked   = "REVOKED"
)

type APIKey struct {
	ID             string     `json:"id"`
	UserID         string     `json:"userId,omitempty"`
	OmniRouteKeyID *string    `json:"omnirouteKeyId,omitempty"`
	KeyHash        string     `json:"-"`
	KeyPrefix      string     `json:"keyPrefix"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	LastUsedAt     *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt      *time.Time `json:"revokedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type PublicAPIKey struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	KeyPrefix       string     `json:"key_prefix"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	LastUsedAt      *time.Time `json:"last_used_at"`
	RevokedAt       *time.Time `json:"revoked_at"`
	OmniRouteLinked bool       `json:"omniroute_linked"`
	LocalDevMode    bool       `json:"local_dev_mode,omitempty"`
}

type AdminAPIKey struct {
	ID             string     `json:"id"`
	KeyPrefix      string     `json:"key_prefix"`
	Status         string     `json:"status"`
	OmniRouteKeyID *string    `json:"omniroute_key_id"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
}

type CreatedAPIKey struct {
	APIKey    PublicAPIKey `json:"api_key"`
	RawAPIKey string       `json:"raw_api_key"`
}

func (k APIKey) Public(localDevMode bool) PublicAPIKey {
	return PublicAPIKey{
		ID:              k.ID,
		Name:            k.Name,
		KeyPrefix:       k.KeyPrefix,
		Status:          k.Status,
		CreatedAt:       k.CreatedAt,
		LastUsedAt:      k.LastUsedAt,
		RevokedAt:       k.RevokedAt,
		OmniRouteLinked: k.OmniRouteKeyID != nil && *k.OmniRouteKeyID != "",
		LocalDevMode:    localDevMode,
	}
}

func (k APIKey) Admin() AdminAPIKey {
	return AdminAPIKey{
		ID:             k.ID,
		KeyPrefix:      k.KeyPrefix,
		Status:         k.Status,
		OmniRouteKeyID: k.OmniRouteKeyID,
		CreatedAt:      k.CreatedAt,
		LastUsedAt:     k.LastUsedAt,
		RevokedAt:      k.RevokedAt,
	}
}
