package enums

import "strings"

type environment string

const (
	development environment = "development"
	staging     environment = "staging"
	production  environment = "production"
)

type Environment environment

type EnvironmentDef struct {
	DEVELOPMENT Environment
	STAGING     Environment
	PRODUCTION  Environment
}

var Environments = &EnvironmentDef{
	DEVELOPMENT: Environment(development),
	STAGING:     Environment(staging),
	PRODUCTION:  Environment(production),
}

func (r Environment) String() string {
	return string(r)
}

func GetEnvironmentFromString(Environment string) Environment {
	switch strings.ToUpper(Environment) {
	case "DEVELOPMENT":
		return Environments.DEVELOPMENT
	case "STAGING":
		return Environments.STAGING
	case "PRODUCTION":
		return Environments.PRODUCTION
	default:
		return Environments.DEVELOPMENT
	}
}

func IsEnvironmentValid(Environment string) bool {
	switch strings.ToUpper(Environment) {
	case "DEVELOPMENT":
		return true
	case "STAGING":
		return true
	case "PRODUCTION":
		return true
	default:
		return false
	}
}
