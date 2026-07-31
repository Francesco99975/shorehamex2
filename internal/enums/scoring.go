package enums

import "strings"

type scoring_method string

const (
	sumBanded         scoring_method = "SUM_BANDED"
	multiscaleProfile scoring_method = "MULTISCALE_PROFILE"
)

type ScoringMethod scoring_method

type ScoringMethodDef struct {
	SUM_BANDED         ScoringMethod
	MULTISCALE_PROFILE ScoringMethod
}

var ScoringMethods = &ScoringMethodDef{
	SUM_BANDED:         ScoringMethod(sumBanded),
	MULTISCALE_PROFILE: ScoringMethod(multiscaleProfile),
}

func (r ScoringMethod) String() string {
	return string(r)
}

func GetScoringMethodFromString(ScoringMethod string) ScoringMethod {
	switch strings.ToUpper(ScoringMethod) {
	case string(sumBanded):
		return ScoringMethods.SUM_BANDED
	case string(multiscaleProfile):
		return ScoringMethods.MULTISCALE_PROFILE
	default:
		return ScoringMethods.SUM_BANDED
	}
}

func IsScoringMethodValid(ScoringMethod string) bool {
	switch strings.ToUpper(ScoringMethod) {
	case string(sumBanded):
		return true
	case string(multiscaleProfile):
		return true
	default:
		return false
	}
}
