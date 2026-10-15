package results

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Francesco99975/shorehamex2/internal/models"
)

// ---------- Configurable T-score bands ----------

// TScoreBands drives the profile chart, meters, level badges and the
// "elevated" filter. Zero value => DefaultTScoreBands() via OrDefault().
type TScoreBands struct {
	Low      int32 // T < Low            -> Low
	Elevated int32 // Elevated <= T < Marked -> Elevated (clinical cutoff)
	Marked   int32 // T >= Marked        -> Markedly elevated
	ChartMin int32 // bottom of the chart/meter scale
	ChartMax int32 // top of the chart/meter scale
	Mean     int32 // dashed reference line
}

func DefaultTScoreBands() TScoreBands {
	return TScoreBands{Low: 40, Elevated: 65, Marked: 75, ChartMin: 30, ChartMax: 120, Mean: 50}
}

func (b TScoreBands) OrDefault() TScoreBands {
	if b == (TScoreBands{}) {
		return DefaultTScoreBands()
	}
	return b
}

type TScoreLevel int

const (
	TScoreNormal TScoreLevel = iota
	TScoreLow
	TScoreElevated
	TScoreMarked
)

func (b TScoreBands) Level(t int32) TScoreLevel {
	switch {
	case t >= b.Marked:
		return TScoreMarked
	case t >= b.Elevated:
		return TScoreElevated
	case t < b.Low:
		return TScoreLow
	default:
		return TScoreNormal
	}
}

func (b TScoreBands) IsElevated(t int32) bool { return t >= b.Elevated }

// Pct maps a T-score onto 0..100 of the chart range (min 2% so tiny bars stay visible).
func (b TScoreBands) Pct(t int32) float64 {
	span := float64(b.ChartMax - b.ChartMin)
	if span <= 0 {
		return 0
	}
	return clamp(float64(t-b.ChartMin)/span*100, 2, 100)
}

func (l TScoreLevel) Label() string {
	switch l {
	case TScoreMarked:
		return "Markedly elevated"
	case TScoreElevated:
		return "Elevated"
	case TScoreLow:
		return "Low"
	default:
		return "Within normal limits"
	}
}

func (l TScoreLevel) BarClass() string {
	switch l {
	case TScoreMarked:
		return "bg-error"
	case TScoreElevated:
		return "bg-warning"
	case TScoreLow:
		return "bg-info"
	default:
		return "bg-success"
	}
}

func (l TScoreLevel) BadgeClass() string {
	switch l {
	case TScoreMarked:
		return "bg-error/10 text-error"
	case TScoreElevated:
		return "bg-warning/15 text-warning"
	case TScoreLow:
		return "bg-info/10 text-info"
	default:
		return "bg-success/10 text-success"
	}
}

type LegendEntry struct {
	Level TScoreLevel
	Range string
}

func (b TScoreBands) Legend() []LegendEntry {
	return []LegendEntry{
		{TScoreLow, fmt.Sprintf("<%d", b.Low)},
		{TScoreNormal, fmt.Sprintf("%d–%d", b.Low, b.Elevated-1)},
		{TScoreElevated, fmt.Sprintf("%d–%d", b.Elevated, b.Marked-1)},
		{TScoreMarked, fmt.Sprintf("≥%d", b.Marked)},
	}
}

// ---------- Category helpers ----------

// IsRawCategory: critical-item categories hold raw endorsed-item counts, not T-scores.
func IsRawCategory(c models.MMPICategoryResult) bool {
	return strings.Contains(strings.ToLower(c.Title), "critical items")
}

var shortTitleSuffix = regexp.MustCompile(`(?i)\s+(scales|critical items)$`)

func ShortTitle(title string) string { return shortTitleSuffix.ReplaceAllString(title, "") }

// HasPurpose filters scorer placeholders ("N/A", "TBA").
func HasPurpose(s models.ScaleResult) bool {
	p := strings.ToUpper(strings.TrimSpace(s.ScalePurpose))
	return p != "" && p != "N/A" && p != "TBA"
}

// UniqueIndications de-duplicates while preserving scorer order.
func UniqueIndications(c models.MMPICategoryResult) []string {
	seen := make(map[string]struct{}, len(c.DerivedIndications))
	out := make([]string, 0, len(c.DerivedIndications))
	for _, ind := range c.DerivedIndications {
		ind = strings.TrimSpace(ind)
		if ind == "" {
			continue
		}
		if _, ok := seen[ind]; ok {
			continue
		}
		seen[ind] = struct{}{}
		out = append(out, ind)
	}
	return out
}

func (b TScoreBands) ElevatedCount(scales []models.ScaleResult) int {
	n := 0
	for _, s := range scales {
		if b.IsElevated(s.Score) {
			n++
		}
	}
	return n
}

// TScoredScales flattens every non-raw category.
func TScoredScales(m models.MMPIResults) []models.ScaleResult {
	var out []models.ScaleResult
	for _, c := range m.Categories {
		if !IsRawCategory(c) {
			out = append(out, c.Scales...)
		}
	}
	return out
}

func CriticalItemTotal(m models.MMPIResults) int {
	total := 0
	for _, c := range m.Categories {
		if IsRawCategory(c) {
			for _, s := range c.Scales {
				total += int(s.Score)
			}
		}
	}
	return total
}

// ---------- Scale grouping (subscales nested under parent) ----------

type ScaleGroup struct {
	Scale    models.ScaleResult
	Children []models.ScaleResult
}

func (g ScaleGroup) HasDetails() bool { return len(g.Children) > 0 || HasPurpose(g.Scale) }

func (b TScoreBands) GroupElevated(g ScaleGroup) bool {
	return b.IsElevated(g.Scale.Score) || b.ElevatedCount(g.Children) > 0
}

func (b TScoreBands) AnyGroupElevated(groups []ScaleGroup) bool {
	for _, g := range groups {
		if b.GroupElevated(g) {
			return true
		}
	}
	return false
}

// D-O / D-S (obvious/subtle), D1..D5 (Harris-Lingoes), FRS1 (content components).
var subscaleSuffix = regexp.MustCompile(`^(.+?)(-[OS]|\d+)$`)

// GroupScales nests a scale under its parent when the parent exists in the
// same category. Orphans (parent missing) stay top-level. Order is preserved.
func GroupScales(scales []models.ScaleResult) []ScaleGroup {
	names := make(map[string]bool, len(scales))
	for _, s := range scales {
		names[s.ScaleName] = true
	}
	parentOf := func(name string) string {
		if m := subscaleSuffix.FindStringSubmatch(name); m != nil && names[m[1]] {
			return m[1]
		}
		return ""
	}
	groups := make([]ScaleGroup, 0, len(scales))
	index := make(map[string]int, len(scales))
	for _, s := range scales {
		if parentOf(s.ScaleName) == "" {
			index[s.ScaleName] = len(groups)
			groups = append(groups, ScaleGroup{Scale: s})
		}
	}
	for _, s := range scales {
		p := parentOf(s.ScaleName)
		if p == "" {
			continue
		}
		if i, ok := index[p]; ok {
			groups[i].Children = append(groups[i].Children, s)
		} else {
			index[s.ScaleName] = len(groups)
			groups = append(groups, ScaleGroup{Scale: s})
		}
	}
	return groups
}

// ---------- Basic profile chart ----------

type ProfileGroup struct {
	Title  string
	Scales []models.ScaleResult
}

var (
	validityTitle = regexp.MustCompile(`(?i)inconsistency|validity`)
	clinicalTitle = regexp.MustCompile(`(?i)^clinical`)
)

// BasicProfile: validity/inconsistency scales + the parent clinical scales.
func BasicProfile(m models.MMPIResults) []ProfileGroup {
	var validity, clinical []models.ScaleResult
	for _, c := range m.Categories {
		switch {
		case validityTitle.MatchString(c.Title):
			validity = append(validity, c.Scales...)
		case clinicalTitle.MatchString(c.Title):
			for _, g := range GroupScales(c.Scales) {
				clinical = append(clinical, g.Scale)
			}
		}
	}
	out := make([]ProfileGroup, 0, 2)
	if len(validity) > 0 {
		out = append(out, ProfileGroup{Title: "Validity", Scales: validity})
	}
	if len(clinical) > 0 {
		out = append(out, ProfileGroup{Title: "Clinical", Scales: clinical})
	}
	return out
}

func CategoryAnchor(i int) string { return fmt.Sprintf("mmpi-cat-%d", i) }
