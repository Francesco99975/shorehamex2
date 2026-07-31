package enums

import "strings"

type BoxDef struct {
	BELOW    Box
	REPLACE  Box
	ABOVE    Box
	TOAST_TR Box
	TOAST_TM Box
	TOAST_TL Box
	TOAST_BR Box
	TOAST_BM Box
	TOAST_BL Box
}

type Box string

var Boxes = &BoxDef{
	BELOW:    Box("BELOW"),
	REPLACE:  Box("REPLACE"),
	ABOVE:    Box("ABOVE"),
	TOAST_TR: Box("TOAST_TR"),
	TOAST_TM: Box("TOAST_TM"),
	TOAST_TL: Box("TOAST_TL"),
	TOAST_BR: Box("TOAST_BR"),
	TOAST_BM: Box("TOAST_BM"),
	TOAST_BL: Box("TOAST_BL"),
}

func (r Box) String() string {
	return string(r)
}

func GetBoxFromString(Box string) Box {
	switch strings.ToUpper(Box) {
	case "BELOW":
		return Boxes.BELOW
	case "REPLACE":
		return Boxes.REPLACE
	case "ABOVE":
		return Boxes.ABOVE
	case "TOAST_TR":
		return Boxes.TOAST_TR
	case "TOAST_TM":
		return Boxes.TOAST_TM
	case "TOAST_TL":
		return Boxes.TOAST_TL
	case "TOAST_BR":
		return Boxes.TOAST_BR
	case "TOAST_BM":
		return Boxes.TOAST_BM
	case "TOAST_BL":
		return Boxes.TOAST_BL
	default:
		return Boxes.BELOW
	}
}

func IsBoxValid(Box string) bool {
	switch strings.ToUpper(Box) {
	case "BELOW":
		return true
	case "REPLACE":
		return true
	case "ABOVE":
		return true
	case "TOAST_TR":
		return true
	case "TOAST_TM":
		return true
	case "TOAST_TL":
		return true
	case "TOAST_BR":
		return true
	case "TOAST_BM":
		return true
	case "TOAST_BL":
		return true
	default:
		return false
	}
}
