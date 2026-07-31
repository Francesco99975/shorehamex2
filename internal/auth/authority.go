package auth

import "github.com/Francesco99975/shorehamex2/internal/enums"

// CanManageUser decides whether `actor` may perform `act` on a user with role `targetRole`.
// `targetRole` is empty for Create (use the role being created instead).
func CanManageUser(actor enums.Role, act enums.Action, targetRole enums.Role) bool {
	switch actor {
	case enums.Roles.DEVELOPER:
		return true // godmode

	case enums.Roles.ADMIN:
		switch act {
		case enums.ActView:
			return targetRole != enums.Roles.DEVELOPER
		case enums.ActCreate:
			// can create USER + ADMIN, not DEVELOPER
			return targetRole == enums.Roles.USER || targetRole == enums.Roles.ADMIN
		case enums.ActEdit, enums.ActDelete:
			// only USERs
			return targetRole == enums.Roles.USER
		}
	}
	return false
}
