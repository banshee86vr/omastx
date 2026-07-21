package api

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// Night-theme tokens from frontend/src/styles/tokens.css (data-theme=night).
var (
	pdfAbyss    = [3]int{0x14, 0x14, 0x14}
	pdfSounding = [3]int{0x1d, 0x1d, 0x1d}
	pdfShoal    = [3]int{0x27, 0x27, 0x27}
	pdfGridline = [3]int{0x38, 0x38, 0x38}
	pdfFoam     = [3]int{0xed, 0xed, 0xed}
	pdfMist     = [3]int{0xa0, 0xa0, 0xa0}
	pdfBeacon  = [3]int{0xe3, 0xff, 0x2f}
	pdfCurrent = [3]int{0x58, 0xc9, 0x8e}
	pdfCaution  = [3]int{0xff, 0xab, 0x24}
	pdfAlarm    = [3]int{0xff, 0x5f, 0x5f}
	pdfFathom   = [3]int{0x7e, 0x8b, 0x96}
)

func setRGBFill(pdf *fpdf.Fpdf, c [3]int) {
	pdf.SetFillColor(c[0], c[1], c[2])
}

func setRGBDraw(pdf *fpdf.Fpdf, c [3]int) {
	pdf.SetDrawColor(c[0], c[1], c[2])
}

func setRGBText(pdf *fpdf.Fpdf, c [3]int) {
	pdf.SetTextColor(c[0], c[1], c[2])
}

func driftClassColor(class string) [3]int {
	switch strings.ToLower(class) {
	case "current":
		return pdfCurrent
	case "patch", "minor":
		return pdfCaution
	case "major", "deprecated":
		return pdfAlarm
	default:
		return pdfFathom
	}
}

func truncatePDF(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "..."
}

func latestOrDash(item artifactDTO) string {
	if item.Latest == nil || *item.Latest == "" {
		return "-"
	}
	return *item.Latest
}

func writeArtifactsPDF(w io.Writer, items []artifactDTO, generatedAt time.Time) error {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 14)
	pdf.SetTitle("Omastx artifact ledger", false)
	pdf.SetCreator("Omastx", false)

	pageW, pageH := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	contentW := pageW - left - right

	cols := []struct {
		label string
		width float64
		mono  bool
	}{
		{"Kind", contentW * 0.07, false},
		{"Identity", contentW * 0.28, true},
		{"Namespace", contentW * 0.12, true},
		{"Cluster", contentW * 0.12, false},
		{"Installed", contentW * 0.12, true},
		{"Latest", contentW * 0.12, true},
		{"Drift", contentW * 0.10, false},
		{"Behind", contentW * 0.07, true},
	}

	paintPageChrome := func() {
		setRGBFill(pdf, pdfAbyss)
		pdf.Rect(0, 0, pageW, pageH, "F")
		setRGBFill(pdf, pdfBeacon)
		pdf.Rect(0, 0, pageW, 2.5, "F")
	}

	drawHeader := func() {
		pdf.SetY(10)
		pdf.SetX(left)
		pdf.SetFont("Helvetica", "B", 18)
		setRGBText(pdf, pdfFoam)
		pdf.CellFormat(40, 8, "Omastx", "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 9)
		setRGBText(pdf, pdfMist)
		pdf.CellFormat(contentW-40, 8, "Artifact ledger export", "", 1, "R", false, 0, "")

		pdf.SetX(left)
		pdf.SetFont("Helvetica", "", 8)
		setRGBText(pdf, pdfMist)
		meta := fmt.Sprintf("%d artifacts  |  generated %s UTC", len(items), generatedAt.UTC().Format("2006-01-02 15:04"))
		pdf.CellFormat(contentW, 5, meta, "", 1, "L", false, 0, "")
		pdf.Ln(3)
	}

	drawTableHeader := func() {
		rowH := 7.0
		setRGBFill(pdf, pdfSounding)
		setRGBDraw(pdf, pdfGridline)
		pdf.SetLineWidth(0.2)
		pdf.SetFont("Helvetica", "B", 8)
		setRGBText(pdf, pdfMist)
		pdf.SetX(left)
		for _, col := range cols {
			pdf.CellFormat(col.width, rowH, col.label, "1", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
	}

	pdf.SetHeaderFunc(func() {
		paintPageChrome()
		if pdf.PageNo() == 1 {
			drawHeader()
		} else {
			pdf.SetY(8)
			pdf.SetX(left)
			pdf.SetFont("Helvetica", "B", 10)
			setRGBText(pdf, pdfFoam)
			pdf.CellFormat(contentW*0.5, 6, "Omastx", "", 0, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 8)
			setRGBText(pdf, pdfMist)
			pdf.CellFormat(contentW*0.5, 6, fmt.Sprintf("page %d", pdf.PageNo()), "", 1, "R", false, 0, "")
			pdf.Ln(2)
		}
		drawTableHeader()
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Helvetica", "", 7)
		setRGBText(pdf, pdfMist)
		pdf.SetX(left)
		pdf.CellFormat(contentW, 5, "Read-only fleet drift | omastx", "", 0, "L", false, 0, "")
		pdf.SetX(left)
		pdf.CellFormat(contentW, 5, fmt.Sprintf("%d / {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AliasNbPages("")

	pdf.AddPage()

	rowH := 6.5
	for i, item := range items {
		if pdf.GetY()+rowH > pageH-14 {
			pdf.AddPage()
		}
		fill := pdfAbyss
		if i%2 == 1 {
			fill = pdfShoal
		}
		setRGBFill(pdf, fill)
		setRGBDraw(pdf, pdfGridline)
		pdf.SetLineWidth(0.15)
		pdf.SetX(left)

		latest := latestOrDash(item)
		behind := ""
		if item.ReleasesBehind != nil {
			behind = fmt.Sprintf("%d", *item.ReleasesBehind)
		}
		kind := item.Kind
		if kind == "helm" {
			kind = "chart"
		}
		values := []string{
			truncatePDF(kind, 12),
			truncatePDF(item.Identity, 48),
			truncatePDF(item.Namespace, 22),
			truncatePDF(item.ClusterName, 22),
			truncatePDF(item.Installed, 20),
			truncatePDF(latest, 20),
			strings.ToUpper(item.DriftClass),
			behind,
		}
		for ci, col := range cols {
			if col.mono {
				pdf.SetFont("Courier", "", 7)
			} else {
				pdf.SetFont("Helvetica", "", 8)
			}
			if ci == 6 {
				setRGBText(pdf, driftClassColor(item.DriftClass))
			} else {
				setRGBText(pdf, pdfFoam)
			}
			pdf.CellFormat(col.width, rowH, values[ci], "1", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
	}

	if len(items) == 0 {
		pdf.SetX(left)
		pdf.SetFont("Helvetica", "", 10)
		setRGBText(pdf, pdfMist)
		pdf.CellFormat(contentW, 12, "No artifacts match the current filters.", "", 1, "L", false, 0, "")
	}

	return pdf.Output(w)
}
