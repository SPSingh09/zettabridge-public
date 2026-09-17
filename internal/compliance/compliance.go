package compliance

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"errors"

	"github.com/SPSingh09/zettabridge/internal/store"
)

var (
	ErrAccountSuspended = errors.New("account suspended")
	ErrOrgSuspended     = errors.New("organization suspended")
)

// UserMayTrade returns an error when the account owner may not place trades.
func UserMayTrade(user *store.User) error {
	if user == nil {
		return ErrAccountSuspended
	}
	status := user.Status
	if status == "" {
		status = domain.StatusActive
	}
	if status == domain.StatusSuspended {
		return ErrAccountSuspended
	}
	return nil
}

// OrgMayTrade returns an error when the org workspace is not active.
func OrgMayTrade(org *store.Organization) error {
	if org == nil {
		return nil
	}
	if org.Status != domain.StatusActive {
		return ErrOrgSuspended
	}
	return nil
}

// WebhookMayTrade checks owner account and org (when set) for live/mock order placement.
func WebhookMayTrade(user *store.User, org *store.Organization) error {
	if err := UserMayTrade(user); err != nil {
		return err
	}
	return OrgMayTrade(org)
}
