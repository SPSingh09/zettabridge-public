package domain

import "testing"

func TestIsValidRole(t *testing.T) {
	for _, r := range []string{RoleOwner, RoleAdmin, RoleMember, RoleViewer} {
		if !IsValidRole(r) {
			t.Fatalf("expected valid role %q", r)
		}
	}
	if IsValidRole("superuser") {
		t.Fatal("unexpected valid role")
	}
}

func TestCanManageOrg(t *testing.T) {
	if !CanManageOrg(RoleOwner) || !CanManageOrg(RoleAdmin) {
		t.Fatal("owner/admin should manage org")
	}
	if CanManageOrg(RoleMember) || CanManageOrg(RoleViewer) {
		t.Fatal("member/viewer should not manage org")
	}
}

func TestHasMinRole(t *testing.T) {
	if !HasMinRole(RoleAdmin, RoleAdmin) {
		t.Fatal("admin should meet admin requirement")
	}
	if HasMinRole(RoleViewer, RoleMember) {
		t.Fatal("viewer should not meet member requirement")
	}
}

func TestCanCreateWebhook(t *testing.T) {
	if !CanCreateWebhook(RoleMember) {
		t.Fatal("member should create webhooks")
	}
	if CanCreateWebhook(RoleViewer) {
		t.Fatal("viewer should not create webhooks")
	}
}

func TestHouseholdPlanRequiredMsg(t *testing.T) {
	msg := HouseholdPlanRequiredMsg("individual")
	if msg != "the individual plan cannot create organizations; upgrade to household" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestCanMutateWebhook(t *testing.T) {
	if !CanMutateWebhook(RoleAdmin, "other", "actor") {
		t.Fatal("admin should mutate any org webhook")
	}
	if !CanMutateWebhook(RoleMember, "self", "self") {
		t.Fatal("member should mutate own webhook")
	}
	if CanMutateWebhook(RoleMember, "other", "self") {
		t.Fatal("member should not mutate other's webhook")
	}
}
