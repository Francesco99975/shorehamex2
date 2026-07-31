package enums

import "strings"

type role string

const (
	developer role = "DEVELOPER"
	admin     role = "ADMIN"
	user      role = "USER"
)

type Role role

type RoleDef struct {
	DEVELOPER Role
	ADMIN     Role
	USER      Role
}

var Roles = &RoleDef{
	DEVELOPER: Role(developer),
	ADMIN:     Role(admin),
	USER:      Role(user),
}

func (r Role) String() string {
	return string(r)
}

func GetRoleFromString(Role string) Role {
	switch strings.ToUpper(Role) {
	case "DEVELOPER":
		return Roles.DEVELOPER
	case "ADMIN":
		return Roles.ADMIN
	case "USER":
		return Roles.USER
	default:
		return Roles.USER
	}
}

func IsRoleValid(Role string) bool {
	switch strings.ToUpper(Role) {
	case "DEVELOPER":
		return true
	case "ADMIN":
		return true
	case "USER":
		return true
	default:
		return false
	}
}
