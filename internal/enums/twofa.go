package enums

import "strings"

type twofaMode string

const (
	registration twofaMode = "totp_registration"
	challenge    twofaMode = "mfa_challenge"
)

type TwofaMode twofaMode

type TwofaModeDef struct {
	REGISTRATION TwofaMode
	CHALLENGE    TwofaMode
}

var TwofaModes = &TwofaModeDef{
	REGISTRATION: TwofaMode(registration),
	CHALLENGE:    TwofaMode(challenge),
}

func (r TwofaMode) String() string {
	return string(r)
}

func GetTwofaModeFromString(mode string) TwofaMode {
	switch strings.ToUpper(mode) {
	case "REGISTRATION":
		return TwofaModes.REGISTRATION
	case "CHALLENGE":
		return TwofaModes.CHALLENGE
	default:
		return TwofaModes.CHALLENGE
	}
}

func IsTwofaModeValid(mode string) bool {
	return mode == TwofaModes.REGISTRATION.String() || mode == TwofaModes.CHALLENGE.String()
}
