// Package admin is the staff back office: roles and permissions, the staff
// authenticator check, and the append-only audit log.
package admin

import "slices"

// Staff roles.
const (
	SuperAdmin = "super_admin"
	Moderator  = "moderator"
	Support    = "support"
	Auditor    = "auditor"
)

var Roles = []string{SuperAdmin, Moderator, Support, Auditor}

// Permissions.
const (
	UsersView        = "users.view"
	UsersEdit        = "users.edit"
	UsersRestrict    = "users.restrict" // freeze, suspend, reinstate
	UsersBan         = "users.ban"
	SellersReview    = "sellers.review"
	ListingsModerate = "listings.moderate"
	ReportsManage    = "reports.manage"
	MessagesView     = "messages.view"
	CasesManage      = "cases.manage"
	FeedbackModerate = "feedback.moderate"
	OrdersView       = "orders.view"
	CatalogEdit      = "catalog.edit"
	FeesEdit         = "fees.edit"
	StaffManage      = "staff.manage"
	AuditView        = "audit.view"
	Announce         = "announce"
)

var rolePerms = map[string][]string{
	SuperAdmin: {UsersView, UsersEdit, UsersRestrict, UsersBan, SellersReview, ListingsModerate, ReportsManage,
		MessagesView, CasesManage, FeedbackModerate, OrdersView, CatalogEdit, FeesEdit, StaffManage, AuditView, Announce},
	Moderator: {UsersView, UsersRestrict, SellersReview, ListingsModerate, ReportsManage, MessagesView,
		FeedbackModerate, OrdersView, Announce},
	Support: {UsersView, UsersEdit, ReportsManage, MessagesView, CasesManage, OrdersView},
	Auditor: {UsersView, OrdersView, AuditView},
}

// Can reports whether a staff role has a permission.
func Can(role, perm string) bool { return slices.Contains(rolePerms[role], perm) }

// Permissions lists what a role may do.
func Permissions(role string) []string {
	out := append([]string{}, rolePerms[role]...)
	return out
}

// ValidRole reports whether r is a staff role.
func ValidRole(r string) bool { return slices.Contains(Roles, r) }
