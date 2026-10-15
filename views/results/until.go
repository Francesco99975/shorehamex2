package results

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/views/ui"
)

// Placeholder rendered for nullable timestamps / missing values.
const Dash = "—"

// Initials returns up to two uppercase initials (first + last word).
func Initials(fullName string) string {
	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return "?"
	}
	first := []rune(parts[0])
	if len(parts) == 1 {
		return strings.ToUpper(string(first[0]))
	}
	last := []rune(parts[len(parts)-1])
	return strings.ToUpper(string(first[0]) + string(last[0]))
}

// AgeAt returns the age in whole years at `now`, birthday-aware.
func AgeAt(dob, now time.Time) int {
	if dob.IsZero() {
		return 0
	}
	age := now.Year() - dob.Year()
	if now.Month() < dob.Month() || (now.Month() == dob.Month() && now.Day() < dob.Day()) {
		age--
	}
	return age
}

// Humanize turns an enum code into a sentence-case label:
// "READY_FOR_REVIEW" -> "Ready for review", "positive_screen" -> "Positive screen".
func Humanize(code string) string {
	s := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), "_", " "))
	if s == "" {
		return Dash
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func FormatDate(t time.Time) string {
	if t.IsZero() {
		return Dash
	}
	return t.Format("Jan 2, 2006")
}

// FormatDateTime formats a nullable timestamptz column; nil renders Dash.
// Callers should convert to the clinic's location before rendering.
func FormatDateTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return Dash
	}
	return t.Format("Jan 2, 2006 · 15:04")
}

// FormatDuration renders seconds as "1h 12m" / "48m" / "19s".
func FormatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	h, m := seconds/3600, (seconds%3600)/60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// TelHref strips formatting from a display phone number ("+1 (416) 789-0099" -> "tel:+14167890099").
func TelHref(phone string) string {
	var b strings.Builder
	for i, r := range phone {
		if unicode.IsDigit(r) || (r == '+' && i == 0) {
			b.WriteRune(r)
		}
	}
	return "tel:" + b.String()
}

// DomID builds a DOM/Alpine-safe identifier from a UUID.
func DomID(prefix, id string) string {
	return prefix + "_" + strings.ReplaceAll(id, "-", "_")
}

// ParsePercent parses headline scores such as "46.43%" into 0..100.
func ParsePercent(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%")), 64)
	if err != nil {
		return 0
	}
	return clamp(v, 0, 100)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------- Status / severity mappings ----------

// AssignmentDone: the patient has submitted the instrument (READY_FOR_REVIEW) or it was reviewed (COMPLETED).
func AssignmentDone(s enums.AssignmentStatus) bool {
	return s == enums.AssignmentStatuses.READY_FOR_REVIEW || s == enums.AssignmentStatuses.COMPLETED
}

func AssignmentStatusVariant(s enums.AssignmentStatus) ui.BadgeVariant {
	switch s {
	case enums.AssignmentStatuses.ASSIGNED:
		return ui.BadgeMuted
	case enums.AssignmentStatuses.IN_PROGRESS:
		return ui.BadgeWarning
	case enums.AssignmentStatuses.READY_FOR_REVIEW:
		return ui.BadgeInfo
	case enums.AssignmentStatuses.COMPLETED:
		return ui.BadgeSuccess
	default:
		return ui.BadgeError
	}
}

func ExaminationStatusVariant(s repository.ExaminationStatus) ui.BadgeVariant {
	switch s {
	case repository.ExaminationStatusASSIGNED:
		return ui.BadgeMuted
	case repository.ExaminationStatusINPROGRESS:
		return ui.BadgeWarning
	case repository.ExaminationStatusCOMPLETED:
		return ui.BadgeSuccess
	default:
		return ui.BadgeError
	}
}

// IsOverdue mirrors the list view rule: past due_at and not yet COMPLETED.
func IsOverdue(dueAt *time.Time, status repository.ExaminationStatus, now time.Time) bool {
	return dueAt != nil && now.After(*dueAt) && status != repository.ExaminationStatusCOMPLETED
}

// Severity labels come from assignment_sum_band_results.severity_label and
// raw_scores[*].gravity (free-text per test definition), so they stay strings.
func SeverityVariant(label string) ui.BadgeVariant {
	switch strings.ToLower(label) {
	case "normal", "minimal", "none":
		return ui.BadgeSuccess
	case "mild":
		return ui.BadgeInfo
	case "moderate":
		return ui.BadgeWarning
	default: // severe, extremely_severe, positive_screen, ...
		return ui.BadgeError
	}
}

func SeverityBarClass(label string) string {
	switch SeverityVariant(label) {
	case ui.BadgeSuccess:
		return "bg-success"
	case ui.BadgeInfo:
		return "bg-info"
	case ui.BadgeWarning:
		return "bg-warning"
	default:
		return "bg-error"
	}
}
