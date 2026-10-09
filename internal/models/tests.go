package models

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

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

type Dass42Content struct {
	Questions []string `json:"questions"`
}

type ScoreBand struct {
	Label      string `json:"label"`
	UpperBound *int   `json:"upperBound"`
}

type Subscale struct {
	Code     string      `json:"code"`
	Name     string      `json:"name"`
	MaxScore int         `json:"maxScore"`
	Adjust   int         `json:"adjust"`
	Bands    []ScoreBand `json:"bands"`
	ItemKeys []int       `json:"itemKeys"`
}

type ScaleResult struct {
	ScaleName        string
	ScaleDescription string
	ScalePurpose     string
	Score            int32
}

type Dass42Definition struct {
	TestDefinition
	Subscales []Subscale    `json:"subscales"`
	Content   Dass42Content `json:"content"`
}

type SubResult struct {
	Score      int
	Gravity    string
	Percentage string
	Indication string
}

func (def *Dass42Definition) GetSubscaleCodeFromItemKey(itemKey int) string {
	for _, subscale := range def.Subscales {
		for _, key := range subscale.ItemKeys {
			if key == itemKey {
				return subscale.Code
			}
		}
	}
	return ""
}

func LoadDass42Definition(path string) (*Dass42Definition, error) {
	var def Dass42Definition
	if err := loadJSON(path, &def); err != nil {
		return nil, fmt.Errorf("load dass42 definition: %w", err)
	}
	return &def, nil
}

type Topic struct {
	Name    string   `json:"name"`
	Answers []string `json:"answers"`
}

type BDI2Content struct {
	Topics []Topic `json:"topics"`
}

type BDI2Definition struct {
	TestDefinition
	Bands   []ScoreBand `json:"bands"`
	Content BDI2Content `json:"content"`
}

func LoadBDI2Definition(path string) (*BDI2Definition, error) {
	var def BDI2Definition
	if err := loadJSON(path, &def); err != nil {
		return nil, fmt.Errorf("load bdi-II definition: %w", err)
	}
	return &def, nil
}

type PTSDContent struct {
	Questions []string `json:"questions"`
}

type PTSDDefinition struct {
	TestDefinition
	Subscales []Subscale  `json:"subscales"`
	Content   PTSDContent `json:"content"`
}

func (def *PTSDDefinition) GetSubscaleCodeFromItemKey(itemKey int) string {
	for _, subscale := range def.Subscales {
		for _, key := range subscale.ItemKeys {
			if key == itemKey {
				return subscale.Code
			}
		}
	}
	return ""
}

func LoadPTSDDefinition(path string) (*PTSDDefinition, error) {
	var def PTSDDefinition
	if err := loadJSON(path, &def); err != nil {
		return nil, fmt.Errorf("load PTSD definition: %w", err)
	}
	return &def, nil
}

type MMPICategoryResult struct {
	Title              string
	Scales             []ScaleResult
	DerivedIndications []string
}

type MMPIResults struct {
	ID         string
	Patient    string
	Sex        string
	Categories []MMPICategoryResult
	Duration   int
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

type MMPIScale struct {
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

func CalculateMMPI(mmpi2Def *MMPIDefinition, answers []string, sex enums.Sex) (MMPIResults, error) {
	var results MMPIResults
	scalesData := mmpi2Def.Content.Scales

	var parsedAnswers []bool

	for _, answer := range strings.Split(strings.Join(answers, ""), "") {
		if answer == "T" {
			parsedAnswers = append(parsedAnswers, true)
		} else {
			parsedAnswers = append(parsedAnswers, false)
		}
	}

	for _, category := range scalesData {

		results.Categories = append(results.Categories, MMPICategoryResult{
			Title:              category.Title,
			Scales:             make([]ScaleResult, 0),
			DerivedIndications: make([]string, 0),
		})

		for _, scale := range category.Items {
			err := grade(parsedAnswers, scale, sex, &results)
			if err != nil {
				return MMPIResults{}, err
			}
		}

		results.Categories[len(results.Categories)-1].deriveIndications(scalesData, &results)
	}

	return results, nil
}

func grade(answers []bool, scale Scale, sex enums.Sex, results *MMPIResults) error {
	if scale.Gender != sex.String() && len(scale.Gender) > 0 {
		return nil
	}

	rawScore := scale.BaseScore

	for _, asw := range scale.Answers {
		for i := 0; i < len(asw); i += 2 {
			if i+1 < len(asw) {
				question := int(asw[i].(float64))
				answer := asw[i+1].(bool)

				if answers[question-1] == answer {
					if len(asw)%2 == 1 {
						rawScore += int32(asw[len(asw)-1].(float64))
					} else {
						rawScore++
					}
				}
			}
		}

	}

	for _, sbs := range scale.SubScales {
		err := grade(answers, sbs, sex, results)

		if err != nil {
			return err
		}
	}

	var finalScore int32

	if sex == enums.Sexes.MALE {
		if len(scale.TScores.Male) > 0 {
			finalScore = calculateTScore(rawScore, scale, sex, results)
		} else {
			finalScore = rawScore
		}
	} else {
		if len(scale.TScores.Female) > 0 {
			finalScore = calculateTScore(rawScore, scale, sex, results)
		} else {
			finalScore = rawScore
		}
	}

	var purpose string
	if len(scale.Text) > 0 {
		purpose = scale.Text
	} else if len(scale.Comment) > 0 {
		purpose = scale.Comment
	} else {
		purpose = "N/A"
	}

	results.Categories[len(results.Categories)-1].Scales = append(results.Categories[len(results.Categories)-1].Scales, ScaleResult{ScaleName: scale.Name, ScaleDescription: scale.Title, ScalePurpose: purpose, Score: finalScore})

	return nil
}

func calculateTScore(rawScore int32, scale Scale, sex enums.Sex, results *MMPIResults) int32 {
	var tScores []int32

	if sex == enums.Sexes.MALE {
		tScores = scale.TScores.Male
	} else {
		tScores = scale.TScores.Female
	}

	if scale.KCorrection > 0.0 {
		var K float32
		var err error
		for _, ct := range results.Categories {
			for _, sr := range ct.Scales {
				if sr.ScaleName == "K" {
					K = float32(sr.Score)
					err = nil
					break
				}
				err = fmt.Errorf("no indications")
			}
		}
		if err != nil {
			K = 1 * scale.KCorrection
		}

		kScore := K + float32(rawScore)

		rawScore = int32(math.Floor(float64(kScore) + 0.5))
	}

	if sex == "male" {
		rawScore -= scale.ScoreOffsets.Male
	} else {
		rawScore -= scale.ScoreOffsets.Female
	}

	return tScores[int(math.Max(0, math.Min(float64(rawScore), float64(len(tScores)-1))))]

}

func (cr *MMPICategoryResult) deriveIndications(scalesData []MMPIScale, currentResults *MMPIResults) {
	var indres []string = make([]string, 0)

	for _, sr := range cr.Scales {
		var indications Indications

		for _, category := range scalesData {
			for _, item := range category.Items {
				if item.Name == sr.ScaleName {
					indications = item.Indications
					break
				}

			}
		}

		var F int32 = 0
		var VRIN int32 = 0
		var TRIN int32 = 0
		for _, ct := range currentResults.Categories {
			for _, sv := range ct.Scales {
				if sv.ScaleName == "F" {
					F = sr.Score

				}

				if sv.ScaleName == "VRIN" {
					VRIN = sr.Score

				}

				if sv.ScaleName == "TRIN" {
					TRIN = sr.Score

				}
			}
		}

		switch sr.ScaleName {
		case "VRIN":
			if sr.Score >= 80 {
				indres = append(indres, indications.VRIN__80...)
			}

			if sr.Score < 40 {
				indres = append(indres, indications.VRIN_40...)
			}

		case "TRIN":
			if sr.Score <= -80 || sr.Score >= 80 {
				indres = append(indres, indications.TRIN___80____TRIN__80...)
			}

		case "Hs":
			if sr.Score >= 75 {
				indres = append(indres, indications.Hs__75...)
			}

			if sr.Score >= 55 && sr.Score <= 74 {
				indres = append(indres, indications.Five5__Hs__74...)
			}

			if sr.Score >= 65 && sr.Score <= 74 {
				indres = append(indres, indications.Six5__Hs__74...)
			}

			if sr.Score >= 55 && sr.Score <= 64 {
				indres = append(indres, indications.Five5__Hs__64...)
			}

		case "Fb":
			if sr.Score > F+20 {
				indres = append(indres, indications.Fb_F_20...)
			}

		case "Fp":
			if sr.Score >= 100 && VRIN < 70 && TRIN > 70 {
				indres = append(indres, indications.Fp__100____VRIN_70____TRIN_70)
			}

		case "D":
			if sr.Score >= 75 {
				indres = append(indres, indications.D__75...)
			}

			if sr.Score >= 65 && sr.Score <= 74 {
				indres = append(indres, indications.Six5__D__74...)
			}

		case "Hy":
			if sr.Score >= 75 {
				indres = append(indres, indications.Hy__75...)
			}

			if sr.Score >= 65 && sr.Score <= 74 {
				indres = append(indres, indications.Six5__Hy__74...)
			}

			if sr.Score >= 55 && sr.Score <= 64 {
				indres = append(indres, indications.Five5__Hy__64...)
			}

		case "Pd":
			if sr.Score >= 75 {
				indres = append(indres, indications.Pd__75...)
			}

			if sr.Score >= 65 && sr.Score <= 74 {
				indres = append(indres, indications.Six5__Pd__74...)
			}

			if sr.Score >= 55 && sr.Score <= 64 {
				indres = append(indres, indications.Five5__Pd__64...)
			}

		case "Mf":
			if sr.Score >= 65 {
				indres = append(indres, indications.Mf__65...)
			}

			if sr.Score >= 45 && sr.Score <= 64 {
				indres = append(indres, indications.Four5__Mf__64...)
			}

			if sr.Score < 45 {
				indres = append(indres, indications.Mf_45...)
			}

		case "Pa":
			if sr.Score >= 75 {
				indres = append(indres, indications.Pa__75...)
			}

			if sr.Score >= 65 && sr.Score <= 74 {
				indres = append(indres, indications.Six5__Pa__74...)
			}

			if sr.Score >= 55 && sr.Score <= 64 {
				indres = append(indres, indications.Five5__Pa__64...)
			}

		case "Pt":
			if sr.Score >= 75 {
				indres = append(indres, indications.Pt__75...)
			}

			if sr.Score >= 55 && sr.Score <= 64 {
				indres = append(indres, indications.Five5__Pt__64...)
			}

		case "Sc":
			if sr.Score >= 75 {
				indres = append(indres, indications.Sc__75...)
			}

			if sr.Score >= 65 && sr.Score <= 74 {
				indres = append(indres, indications.Six5__Sc__74...)
			}

			if sr.Score >= 55 && sr.Score <= 64 {
				indres = append(indres, indications.Five5__Sc__64...)
			}

		case "Ma":
			if sr.Score >= 75 {
				indres = append(indres, indications.Ma__75...)
			}

			if sr.Score >= 65 && sr.Score <= 74 {
				indres = append(indres, indications.Ma__65__74...)
			}

			if sr.Score >= 55 && sr.Score <= 64 {
				indres = append(indres, indications.Ma__55__64...)
			}

		case "Si":
			if sr.Score >= 75 {
				indres = append(indres, indications.Si__75...)
			}

			if sr.Score >= 65 && sr.Score <= 74 {
				indres = append(indres, indications.Si__65__74...)
			}

			if sr.Score >= 55 && sr.Score <= 64 {
				indres = append(indres, indications.Si__55__64...)
			}

			if sr.Score < 45 {
				indres = append(indres, indications.Si_45...)
			}

		}
	}

	if len(indres) > 0 {
		cr.DerivedIndications = append(cr.DerivedIndications, indres...)
	} else {
		cr.DerivedIndications = append(cr.DerivedIndications, "No particular indications")
	}

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
