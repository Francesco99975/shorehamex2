package tools

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Francesco99975/shorehamex2/internal/models"
	"github.com/Francesco99975/shorehamex2/internal/repository"
	"github.com/Francesco99975/shorehamex2/views/results"
	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

func newPDFGenerator() core.Maroto {
	cfg := config.NewBuilder().
		WithPageNumber().
		WithLeftMargin(10).
		WithTopMargin(15).
		WithRightMargin(10).
		Build()

	return maroto.NewMetricsDecorator(maroto.New(cfg))
}

func savePDF(m core.Maroto, filename string) (string, error) {
	document, err := m.Generate()
	if err != nil {
		return "", fmt.Errorf("generate PDF: %w", err)
	}

	if err := document.Save(filename); err != nil {
		return "", fmt.Errorf("save PDF: %w", err)
	}

	return filename, nil
}

func pdfFilename(name, id string) string {
	// Use a filesystem-safe timestamp rather than a formatted date
	// containing spaces and timezone text.
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	id = strings.ReplaceAll(id, string(filepath.Separator), "_")
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "\\", "_")

	return fmt.Sprintf("%s_%s_%s.pdf", name, id, timestamp)
}

type PatientDetails struct {
	MRN  string
	Name string
	Sex  string
}

func GeneratePDFGeneric(
	test *repository.TestDefinition,
	patientDetails PatientDetails,
	sumBands []results.SumBand,
	duration int,
) (string, error) {
	m := newPDFGenerator()
	date := time.Now().Format(time.DateTime)

	if err := m.RegisterHeader(
		getPageHeader(patientDetails.MRN, patientDetails.Name, patientDetails.Sex, duration, date),
	); err != nil {
		return "", fmt.Errorf("register PDF header: %w", err)
	}

	m.AddRows(text.NewRow(25,
		fmt.Sprintf("%s results for %s", test.Name, patientDetails.Name),
		props.Text{
			Top:   3,
			Style: fontstyle.Bold,
			Align: align.Center,
			Size:  20,
		},
	))

	for _, band := range sumBands {
		m.AddRows(
			text.NewRow(25,
				fmt.Sprintf("%s", band.SubscaleCode),
				props.Text{
					Top:   3,
					Style: fontstyle.Bold,
					Align: align.Center,
					Size:  20,
				},
			),
			text.NewRow(30,
				fmt.Sprintf("Score: %s", band.HeadlineScore),
				props.Text{
					Top:   5,
					Style: fontstyle.Italic,
					Align: align.Center,
					Size:  16,
				},
			),
			text.NewRow(30,
				fmt.Sprintf("Indication: %s", band.InterpretationSummary),
				props.Text{
					Top:   5,
					Style: fontstyle.Italic,
					Align: align.Center,
					Size:  16,
				},
			),
		)
	}

	return savePDF(m, pdfFilename(test.Name, patientDetails.MRN))
}

func GeneratePDFMMPI(results models.MMPIResults) (string, error) {
	m := newPDFGenerator()
	date := time.Now().Format(time.DateTime)
	darkGray := getDarkGrayColor()

	if err := m.RegisterHeader(
		getPageHeader(
			results.ID,
			results.Patient,
			results.Sex,
			results.Duration,
			date,
		),
	); err != nil {
		return "", fmt.Errorf("register PDF header: %w", err)
	}

	m.AddRows(
		text.NewRow(25,
			fmt.Sprintf("MMPI-2 results for %s", results.Patient),
			props.Text{
				Top:   3,
				Style: fontstyle.Bold,
				Align: align.Center,
				Size:  20,
			},
		),
	)

	for _, category := range results.Categories {
		m.AddRows(
			text.NewRow(20, category.Title, props.Text{
				Top:   1.5,
				Style: fontstyle.Bold,
				Align: align.Center,
				Size:  16,
				Color: &props.WhiteColor,
			}).WithStyle(&props.Cell{
				BackgroundColor: darkGray,
			}),
		)

		m.AddRows(getTransactions(category.Scales)...)

		for i := 0; i < len(category.DerivedIndications); i += 3 {
			end := min(i+3, len(category.DerivedIndications))
			content := strings.Join(
				category.DerivedIndications[i:end],
				", ",
			)

			if i == 0 {
				content = "Indications: " + content
			}

			m.AddRows(
				text.NewRow(20, content, props.Text{
					Top:   5,
					Style: fontstyle.Italic,
					Align: align.Left,
					Size:  12,
					Color: getBlueColor(),
				}),
			)
		}
	}

	return savePDF(m, pdfFilename("MMPI-2", results.ID))
}

func getTransactions(scaleResults []models.ScaleResult) []core.Row {
	rows := []core.Row{
		row.New(15).Add(
			text.NewCol(2, "Scale", props.Text{
				Size: 12, Align: align.Center, Style: fontstyle.Bold,
			}),
			text.NewCol(4, "Scale Description", props.Text{
				Size: 12, Align: align.Center, Style: fontstyle.Bold,
			}),
			text.NewCol(4, "Scale Purpose", props.Text{
				Size: 12, Align: align.Center, Style: fontstyle.Bold,
			}),
			text.NewCol(2, "Scale Score", props.Text{
				Size: 12, Align: align.Center, Style: fontstyle.Bold,
			}),
		),
	}

	for i, scale := range scaleResults {
		r := row.New(15).Add(
			text.NewCol(2, scale.ScaleName, props.Text{
				Size: 12, Align: align.Center,
			}),
			text.NewCol(4, scale.ScaleDescription, props.Text{
				Size: 12, Align: align.Center,
			}),
			text.NewCol(4, scale.ScalePurpose, props.Text{
				Size: 12, Align: align.Center,
			}),
			text.NewCol(2, strconv.Itoa(int(scale.Score)), props.Text{
				Size: 12, Align: align.Center,
			}),
		)

		if i%2 == 0 {
			r = r.WithStyle(&props.Cell{
				BackgroundColor: getGrayColor(),
			})
		}

		rows = append(rows, r)
	}

	return rows
}

func getPageHeader(
	id string,
	patient string,
	sex string,
	duration int,
	date string,
) core.Row {
	totalSeconds := duration
	durationMin := totalSeconds / 60
	durationSec := totalSeconds % 60

	return row.New(50).Add(
		col.New(10).Add(
			text.New(fmt.Sprintf("ID: %s", id), props.Text{
				Size:  12,
				Align: align.Left,
				Style: fontstyle.BoldItalic,
				Color: getBlueColor(),
			}),
			text.New(fmt.Sprintf("Patient: %s", patient), props.Text{
				Size:  12,
				Top:   6,
				Align: align.Left,
				Style: fontstyle.BoldItalic,
				Color: getBlueColor(),
			}),
			text.New(fmt.Sprintf("Sex: %s", sex), props.Text{
				Top:   12,
				Size:  12,
				Align: align.Left,
				Color: getBlueColor(),
			}),
			text.New(fmt.Sprintf("Date: %s", date), props.Text{
				Top:   18,
				Size:  12,
				Align: align.Left,
				Color: getBlueColor(),
			}),
			text.New(
				fmt.Sprintf(
					"Taken in: %d minutes and %d seconds",
					durationMin,
					durationSec,
				),
				props.Text{
					Top:   24,
					Size:  12,
					Align: align.Left,
					Color: getBlueColor(),
				},
			),
		),
	)
}

func getDarkGrayColor() *props.Color {
	return &props.Color{Red: 55, Green: 55, Blue: 55}
}

func getGrayColor() *props.Color {
	return &props.Color{Red: 200, Green: 200, Blue: 200}
}

func getBlueColor() *props.Color {
	return &props.Color{Red: 10, Green: 10, Blue: 150}
}
