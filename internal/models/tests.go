package models

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Francesco99975/shorehamex2/internal/enums"
)

type Parameter struct {
	Label string `json:"label"`
	Note  string `json:"note"`
}

type Severity string

const (
	SeverityNormal   Severity = "normal"
	SeverityModerate Severity = "moderate"
	SeveritySevere   Severity = "severe"
)

type TestDefinition struct {
	Name               string              `json:"name"`
	Code               string              `json:"code"`
	ShortDescription   string              `json:"shortDescription"`
	Purpose            string              `json:"purpose"`
	Population         string              `json:"population"`
	ScoringDescription string              `json:"scoringDescription"`
	Parameters         []Parameter         `json:"parameters"`
	MaxScore           int                 `json:"maxScore"`
	RequiresSex        bool                `json:"requires_sex"`
	TestDomain         enums.TestDomain    `json:"test_domain"`
	ScoringMethod      enums.ScoringMethod `json:"scoring_method"`
	TypicalDurationMin int                 `json:"typical_duration_min"`
	// Adjust is subtracted from the raw summed score before banding.
	// Zero for every test except P3.
	Adjust int `json:"adjust"`
	// Low/High are the raw (post-adjust) score thresholds:
	//   score <= Low            -> normal
	//   Low  <  score <= High    -> moderate
	//   score  > High            -> severe
	Low   int `json:"low"`
	High  int `json:"high"`
	Items int `json:"items"`
}

// AdjustedScore subtracts Adjust from a raw summed score. For every test
// but P3, Adjust is 0 and this is a no-op.
func (d TestDefinition) AdjustedScore(rawScore int) int {
	return rawScore - d.Adjust
}

// Percentage returns the adjusted score as a percentage of MaxScore,
// matching CompileBasicIndication's "%.2f%%" display value.
func (d TestDefinition) Percentage(rawScore int) float64 {
	adjusted := d.AdjustedScore(rawScore)
	return float64(adjusted) / float64(d.MaxScore) * 100.0
}

// Severity bands an adjusted score using contiguous (bug-free)
// boundaries: score <= Low -> normal, Low < score <= High -> moderate,
// otherwise severe. Pass the *raw* score; Severity applies AdjustedScore
// itself, so callers don't need to subtract Adjust beforehand.
func (d TestDefinition) Severity(rawScore int) Severity {
	adjusted := d.AdjustedScore(rawScore)
	switch {
	case adjusted <= d.Low:
		return SeverityNormal
	case adjusted <= d.High:
		return SeverityModerate
	default:
		return SeveritySevere
	}
}

type TestDefinitionView struct {
	TestDefinition
	AssignmentCount int `json:"assignmentCount"`
}

// ---------------------------------------------------------------------
// ASQ — 5 single-choice items ("questions") + 33 checkbox items
// ("multiq"), each worth 1 point if endorsed.
// ---------------------------------------------------------------------

type AsqContent struct {
	Questions []string `json:"questions"`
	Multiq    []string `json:"multiq"`
}

type AsqDefinition struct {
	TestDefinition
	Content AsqContent `json:"content"`
}

func LoadAsqDefinition(path string) (*AsqDefinition, error) {
	var def AsqDefinition
	if err := loadJSON(path, &def); err != nil {
		return nil, fmt.Errorf("load asq definition: %w", err)
	}
	return &def, nil
}

// ---------------------------------------------------------------------
// BAI — 21 flat items, each answered on a 0-3 scale.
// ---------------------------------------------------------------------

type BaiContent struct {
	Questions []string `json:"questions"`
}

type BaiDefinition struct {
	TestDefinition
	Content BaiContent `json:"content"`
}

func LoadBaiDefinition(path string) (*BaiDefinition, error) {
	var def BaiDefinition
	if err := loadJSON(path, &def); err != nil {
		return nil, fmt.Errorf("load bai definition: %w", err)
	}
	return &def, nil
}

// ---------------------------------------------------------------------
// BDI and P3 — both use grouped items: each item is a list of option
// strings, where the chosen option's array index is (or, for P3,
// index+1 is) its point value. Same shape, so one content type covers
// both.
// ---------------------------------------------------------------------

// GroupedOptionsContent holds items that are each a list of selectable
// option strings (used by both BDI and P3).
type GroupedOptionsContent struct {
	Questions [][]string `json:"questions"`
}

type BdiDefinition struct {
	TestDefinition
	Content GroupedOptionsContent `json:"content"`
}

func LoadBdiDefinition(path string) (*BdiDefinition, error) {
	var def BdiDefinition
	if err := loadJSON(path, &def); err != nil {
		return nil, fmt.Errorf("load bdi definition: %w", err)
	}
	return &def, nil
}

type P3Definition struct {
	TestDefinition
	Content GroupedOptionsContent `json:"content"`
}

func LoadP3Definition(path string) (*P3Definition, error) {
	var def P3Definition
	if err := loadJSON(path, &def); err != nil {
		return nil, fmt.Errorf("load p3 definition: %w", err)
	}
	return &def, nil
}

type Indications struct {
	Four5__Mf__64                 []string `json:"45<=Mf<=64"`
	Five5__Hs__64                 []string `json:"55<=Hs<=64"`
	Five5__Hs__74                 []string `json:"55<=Hs<=74"`
	Five5__Hy__64                 []string `json:"55<=Hy<=64"`
	Five5__Pa__64                 []string `json:"55<=Pa<=64"`
	Five5__Pd__64                 []string `json:"55<=Pd<=64"`
	Five5__Pt__64                 []string `json:"55<=Pt<=64"`
	Five5__Sc__64                 []string `json:"55<=Sc<=64"`
	Six5__D__74                   []string `json:"65<=D<=74"`
	Six5__Hs__74                  []string `json:"65<=Hs<=74"`
	Six5__Hy__74                  []string `json:"65<=Hy<=74"`
	Six5__Pa__74                  []string `json:"65<=Pa<=74"`
	Six5__Pd__74                  []string `json:"65<=Pd<=74"`
	Six5__Pt__74                  []string `json:"65<=Pt<=74"`
	Six5__Sc__74                  []string `json:"65<=Sc<=74"`
	D__75                         []string `json:"D>=75"`
	Fb_F_20                       []string `json:"Fb>F+20"`
	Fp__100____VRIN_70____TRIN_70 string   `json:"Fp>=100 && VRIN<70 && TRIN<70"`
	Hs__75                        []string `json:"Hs>=75"`
	Hy__75                        []string `json:"Hy>=75"`
	Ma__55__64                    []string `json:"Ma<=55<=64"`
	Ma__65__74                    []string `json:"Ma<=65<=74"`
	Ma__75                        []string `json:"Ma>=75"`
	Mf_45                         []string `json:"Mf<45"`
	Mf__65                        []string `json:"Mf>=65"`
	Pa__75                        []string `json:"Pa>=75"`
	Pd__75                        []string `json:"Pd>=75"`
	Pt__75                        []string `json:"Pt>=75"`
	Sc__75                        []string `json:"Sc>=75"`
	Si_45                         []string `json:"Si<45"`
	Si__55__64                    []string `json:"Si<=55<=64"`
	Si__65__74                    []string `json:"Si<=65<=74"`
	Si__75                        []string `json:"Si>=75"`
	TRIN___80____TRIN__80         []string `json:"TRIN<=-80 || TRIN>=80"`
	VRIN_40                       []string `json:"VRIN<40"`
	VRIN__80                      []string `json:"VRIN>=80"`
}

type Scale struct {
	Answers      [][]interface{} `json:"answers"`
	BaseScore    int32           `json:"baseScore"`
	Code         string          `json:"code"`
	Comment      string          `json:"comment"`
	Gender       string          `json:"gender"`
	Indications  Indications     `json:"indications"`
	KCorrection  float32         `json:"kCorrection"`
	Name         string          `json:"name"`
	ScoreOffsets struct {
		Female int32 `json:"female"`
		Male   int32 `json:"male"`
	} `json:"scoreOffsets"`
	SubScales []Scale `json:"subScales"`
	TScores   struct {
		Female []int32 `json:"female"`
		Male   []int32 `json:"male"`
	} `json:"tScores"`
	Text  string `json:"text"`
	Title string `json:"title"`
}

type MMPIScale []struct {
	Items []Scale `json:"items"`
	Title string  `json:"title"`
}

type MMPIContent struct {
	Questions []string    `json:"questions"`
	Scales    []MMPIScale `json:"scales"`
}

type MMPIDefinition struct {
	TestDefinition
	Content MMPIContent `json:"content"`
}

func LoadMMPIDefinition(path string) (*MMPIDefinition, error) {
	var def MMPIDefinition
	if err := loadJSON(path, &def); err != nil {
		return nil, fmt.Errorf("load mmpi definition: %w", err)
	}
	return &def, nil
}

// ---------------------------------------------------------------------
// shared helper
// ---------------------------------------------------------------------

func loadJSON(path string, target interface{}) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	return nil
}
