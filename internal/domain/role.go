package domain

import "errors"

// Platform-level user roles.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// User and org entity status values.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

// Plan names.
const (
	PlanFree    = "free"
	PlanPaper   = "paper"
	PlanPro     = "pro"
	PlanProPlus = "pro_plus"
)

var (
	ErrInvalidPlan   = errors.New("plan must be free, paper, pro, or pro_plus")
	ErrInvalidStatus = errors.New("status must be active or suspended")
	ErrForbidden     = errors.New("platform admin access required")
)

func IsValidPlan(plan string) bool {
	switch plan {
	case PlanFree, PlanPaper, PlanPro, PlanProPlus:
		return true
	default:
		return false
	}
}

func IsValidStatus(status string) bool {
	return status == StatusActive || status == StatusSuspended
}

func IsPlatformAdmin(r string) bool {
	return r == RoleAdmin
}
