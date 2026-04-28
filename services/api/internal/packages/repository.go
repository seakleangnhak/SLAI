package packages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var ErrInvalidPackage = errors.New("invalid package")

type Repository struct {
	db platformdb.Executor
}

func NewRepository(db platformdb.Executor) Repository {
	return Repository{db: db}
}

func (r Repository) ListActive(ctx context.Context) ([]Package, error) {
	return r.list(ctx, `WHERE active = true`)
}

func (r Repository) ListAll(ctx context.Context) ([]Package, error) {
	return r.list(ctx, ``)
}

func (r Repository) Create(ctx context.Context, input CreatePackageInput) (Package, error) {
	if strings.TrimSpace(input.Name) == "" || input.CreditUnits <= 0 || input.BonusCreditUnits < 0 || input.PriceMinor < 0 {
		return Package{}, ErrInvalidPackage
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}

	return scanPackage(r.db.QueryRow(ctx, `
		INSERT INTO credit_packages (name, description, credit_units, bonus_credit_units, price_minor, currency, active, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text, name, description, credit_units, bonus_credit_units, price_minor, currency, active, sort_order, created_at, updated_at
	`, strings.TrimSpace(input.Name), input.Description, input.CreditUnits, input.BonusCreditUnits, input.PriceMinor, strings.ToUpper(input.Currency), active, input.SortOrder))
}

func (r Repository) Update(ctx context.Context, id string, input UpdatePackageInput) (Package, error) {
	if id == "" {
		return Package{}, ErrInvalidPackage
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Package{}, ErrInvalidPackage
	}
	if input.CreditUnits != nil && *input.CreditUnits <= 0 {
		return Package{}, ErrInvalidPackage
	}
	if input.BonusCreditUnits != nil && *input.BonusCreditUnits < 0 {
		return Package{}, ErrInvalidPackage
	}
	if input.PriceMinor != nil && *input.PriceMinor < 0 {
		return Package{}, ErrInvalidPackage
	}

	return scanPackage(r.db.QueryRow(ctx, `
		UPDATE credit_packages
		SET name = COALESCE($2, name),
		    description = COALESCE($3, description),
		    credit_units = COALESCE($4, credit_units),
		    bonus_credit_units = COALESCE($5, bonus_credit_units),
		    price_minor = COALESCE($6, price_minor),
		    currency = COALESCE(UPPER($7), currency),
		    active = COALESCE($8, active),
		    sort_order = COALESCE($9, sort_order)
		WHERE id = $1
		RETURNING id::text, name, description, credit_units, bonus_credit_units, price_minor, currency, active, sort_order, created_at, updated_at
	`, id, input.Name, input.Description, input.CreditUnits, input.BonusCreditUnits, input.PriceMinor, input.Currency, input.Active, input.SortOrder))
}

func (r Repository) Get(ctx context.Context, id string) (Package, error) {
	return scanPackage(r.db.QueryRow(ctx, `
		SELECT id::text, name, description, credit_units, bonus_credit_units, price_minor, currency, active, sort_order, created_at, updated_at
		FROM credit_packages
		WHERE id = $1
	`, id))
}

func (r Repository) list(ctx context.Context, where string) ([]Package, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, description, credit_units, bonus_credit_units, price_minor, currency, active, sort_order, created_at, updated_at
		FROM credit_packages
		`+where+`
		ORDER BY sort_order ASC, price_minor ASC, created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}
	defer rows.Close()

	packages := []Package{}
	for rows.Next() {
		pkg, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate packages: %w", err)
	}
	return packages, nil
}

func scanPackage(row pgx.Row) (Package, error) {
	var pkg Package
	err := row.Scan(
		&pkg.ID,
		&pkg.Name,
		&pkg.Description,
		&pkg.CreditUnits,
		&pkg.BonusCreditUnits,
		&pkg.PriceMinor,
		&pkg.Currency,
		&pkg.Active,
		&pkg.SortOrder,
		&pkg.CreatedAt,
		&pkg.UpdatedAt,
	)
	if err != nil {
		return Package{}, fmt.Errorf("scan package: %w", err)
	}
	return pkg, nil
}
