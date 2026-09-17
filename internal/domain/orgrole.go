package domain

import (
	"errors"
	"fmt"
)

// Org-level roles. RoleAdmin is shared with the platform role in role.go.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
	RoleViewer = "viewer"
)

// OrgStatusActive and OrgStatusSuspended are aliases of StatusActive/StatusSuspended
// kept for compatibility; prefer StatusActive/StatusSuspended directly.
const (
	OrgStatusActive    = StatusActive
	OrgStatusSuspended = StatusSuspended
)

var (
	ErrInvalidRole           = errors.New("role must be owner, admin, member, or viewer")
	ErrInvalidOrgStatus      = errors.New("org status must be active or suspended")
	ErrMemberNotActive       = errors.New("org membership is not active")
	ErrInsufficientRole      = errors.New("insufficient org permissions")
	ErrOrgNotFound           = errors.New("org not found")
	ErrAlreadyInOrg          = errors.New("user already belongs to an organization")
	ErrOrgsNotEnabled        = errors.New("organization creation is not enabled for this account")
	ErrHouseholdRequired     = errors.New("household plan required for organization creation")
	ErrSeatLimitReached      = errors.New("organization seat limit reached")
	ErrInvalidInvite         = errors.New("invite is invalid or expired")
	ErrInviteNotFound        = errors.New("invite not found")
	ErrCannotInviteOwner     = errors.New("cannot invite with owner role")
	ErrCannotModifyOwner     = errors.New("cannot modify organization owner")
	ErrCannotRemoveOwner     = errors.New("cannot remove organization owner")
	ErrAlreadyMember         = errors.New("user is already a member of this organization")
	ErrUserInOtherOrg        = errors.New("user already belongs to another organization")
	ErrPendingInviteExists   = errors.New("a pending invite already exists for this email")
	ErrOwnerCannotLeave      = errors.New("organization owner cannot leave; transfer ownership or delete org")
	ErrSoloResourcesDisabled = errors.New("personal webhooks and credentials are disabled while you belong to an organization")
	ErrCannotTransferToSelf  = errors.New("cannot transfer ownership to yourself")
	ErrTargetNotMember       = errors.New("target user is not an active org member")
	ErrSeatLimitTooLow       = errors.New("seat_limit cannot be less than current seats used")
)

func HouseholdPlanRequiredMsg(currentPlan string) string {
	return fmt.Sprintf("the %s plan cannot create organizations; upgrade to household", currentPlan)
}

func IsValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func IsValidOrgStatus(status string) bool {
	return status == StatusActive || status == StatusSuspended
}

// CanManageOrg returns true for owner and admin.
func CanManageOrg(role string) bool {
	return role == RoleOwner || role == RoleAdmin
}

// IsOwner returns true only for the org owner.
func IsOwner(role string) bool {
	return role == RoleOwner
}

// RoleRank returns a numeric rank; higher = more privilege.
func RoleRank(role string) int {
	switch role {
	case RoleOwner:
		return 4
	case RoleAdmin:
		return 3
	case RoleMember:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func HasMinRole(actual, required string) bool {
	return RoleRank(actual) >= RoleRank(required)
}

// IsInvitableRole is true for roles that may be assigned via invite (never owner).
func IsInvitableRole(role string) bool {
	switch role {
	case RoleAdmin, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func CanManageMembers(role string) bool {
	return CanManageOrg(role)
}

func CanManageCredentials(role string) bool {
	return CanManageOrg(role)
}

func CanCreateWebhook(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember
}

// CanMutateWebhook returns whether the actor may pause/resume/delete the webhook.
func CanMutateWebhook(actorRole, creatorUserID, actorUserID string) bool {
	if actorRole == RoleOwner || actorRole == RoleAdmin {
		return true
	}
	if actorRole == RoleMember {
		return creatorUserID == actorUserID
	}
	return false
}

func CanViewOrgResources(role string) bool {
	return IsValidRole(role)
}
