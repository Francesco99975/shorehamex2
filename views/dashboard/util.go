package dashboard

import "fmt"

// Percent returns part/total as 0–100, clamped. total <= 0 yields 0.
func Percent(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	p := float64(part) / float64(total) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func widthStyle(pct float64) string {
	return fmt.Sprintf("width:%.2f%%", pct)
}

// SignedPercent formats a delta like "+18%" / "−5%" / "0%".
func SignedPercent(delta int) string {
	switch {
	case delta > 0:
		return fmt.Sprintf("+%d%%", delta)
	case delta < 0:
		return fmt.Sprintf("\u2212%d%%", -delta)
	default:
		return "0%"
	}
}

func pluralItems(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
