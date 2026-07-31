package enums

import "strings"

type sex string

const (
	male   sex = "MALE"
	female sex = "FEMALE"
)

type Sex sex

type SexDef struct {
	MALE   Sex
	FEMALE Sex
}

var Sexes = &SexDef{
	MALE:   Sex(male),
	FEMALE: Sex(female),
}

func (r Sex) String() string {
	return string(r)
}

func GetSexFromString(sex string) Sex {
	switch strings.ToUpper(sex) {
	case "MALE":
		return Sexes.MALE
	case "FEMALE":
		return Sexes.FEMALE
	default:
		return Sexes.MALE
	}
}

func IsSexValid(sex string) bool {
	switch strings.ToUpper(sex) {
	case "MALE":
		return true
	case "FEMALE":
		return true
	default:
		return false
	}
}
