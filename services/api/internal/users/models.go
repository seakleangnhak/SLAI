package users

import "time"

const (
	RoleUser  = "USER"
	RoleAdmin = "ADMIN"

	StatusActive    = "ACTIVE"
	StatusSuspended = "SUSPENDED"
)

type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	BalancePolicy string    `json:"balancePolicy"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	PasswordHash  string    `json:"-"`
}

func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u User) IsActive() bool {
	return u.Status == StatusActive
}
