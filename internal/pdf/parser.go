package pdf

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	gopdf "github.com/ledongthuc/pdf"
)

// Deduction represents a single pre-tax deduction extracted from a paystub.
type Deduction struct {
	Type      string  // "401k", "403b", "hsa", "fsa_health", "fsa_dependent", "commuter"
	Amount    float64
	YTDAmount float64
}

// PaystubData holds the extracted data from a paystub PDF.
type PaystubData struct {
	FirstName             string
	GrossPay              float64
	FederalTaxWithheld    float64
	PayPeriodStart        time.Time
	PayPeriodEnd          time.Time
	YTDGrossPay           float64
	YTDFederalTaxWithheld float64
	Hours                 float64
	YTDHours              float64
	Deductions            []Deduction
}

// ParseError represents a clear error from the PDF parser.
type ParseError struct {
	Message string
}

func (e *ParseError) Error() string {
	return e.Message
}

// ParsePaystub extracts paystub fields from a PDF file.
// It only supports text-based PDFs. Image-based PDFs will return an error.
func ParsePaystub(r io.ReaderAt, size int64) (*PaystubData, error) {
	reader, err := gopdf.NewReader(r, size)
	if err != nil {
		return nil, &ParseError{Message: fmt.Sprintf("failed to read PDF: %v", err)}
	}

	var textBuilder strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		textBuilder.WriteString(content)
		textBuilder.WriteString("\n")
	}

	text := textBuilder.String()
	if strings.TrimSpace(text) == "" {
		return nil, &ParseError{Message: "PDF appears to be image-based or contains no extractable text"}
	}

	// Strip sensitive data from text before processing.
	sanitized := StripSensitiveData(text)

	data, err := extractFields(sanitized)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// StripSensitiveData removes SSNs, addresses, bank account numbers, and employer IDs.
func StripSensitiveData(text string) string {
	// SSN patterns: XXX-XX-XXXX or XXXXXXXXX
	ssnPattern := regexp.MustCompile(`\b\d{3}-?\d{2}-?\d{4}\b`)
	text = ssnPattern.ReplaceAllString(text, "[REDACTED]")

	// EIN patterns: XX-XXXXXXX
	einPattern := regexp.MustCompile(`\b\d{2}-\d{7}\b`)
	text = einPattern.ReplaceAllString(text, "[REDACTED]")

	// Bank account numbers: sequences of 8-17 digits that could be account numbers.
	// We're conservative here — only redact things that look like routing/account numbers.
	bankPattern := regexp.MustCompile(`(?i)(?:account|routing|acct)[\s#:]*\d{4,17}`)
	text = bankPattern.ReplaceAllString(text, "[REDACTED]")

	return text
}

var (
	// Common date formats found in paystubs.
	dateFormats = []string{
		"01/02/2006",
		"1/2/2006",
		"01-02-2006",
		"1-2-2006",
		"2006-01-02",
		"Jan 02, 2006",
		"January 2, 2006",
		"01/02/06",
		"1/2/06",
	}

	// Pay period range pattern: "Pay Period: 01/01/2025 - 01/15/2025"
	// Also handles "Period: ..." without "Pay" prefix and dash-separated dates.
	payPeriodPattern = regexp.MustCompile(`(?i)(?:pay\s*)?period[\s:]*(\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4})\s*(?:-|to|through|thru)\s*(\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4})`)

	// Separate begin/end patterns for paystubs with split labels.
	periodBeginPattern = regexp.MustCompile(`(?i)period\s*begin[\s:]*(\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4})`)
	periodEndPattern   = regexp.MustCompile(`(?i)period\s*end(?:ing)?[\s:]*(\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4})`)

	// Pay Date / Check Date fallback (last resort, infer period).
	payDatePattern = regexp.MustCompile(`(?i)(?:pay\s*date|check\s*date)[\s:]*(\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4})`)

	// Label detectors for columnar layouts where labels and values are separated.
	periodBeginLabelPattern = regexp.MustCompile(`(?i)period\s*begin`)
	periodEndLabelPattern   = regexp.MustCompile(`(?i)period\s*end`)

	// Generic date finder for columnar layout extraction.
	anyDatePattern = regexp.MustCompile(`\b(\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4})\b`)

	// Gross pay patterns - handles various payroll provider labels.
	grossPayPattern = regexp.MustCompile(`(?i)(?:gross\s*pay|gross\s*earnings|gross\s*wages|total\s*(?:gross|earnings)|earnings\s*total)[\s:]*\$?([0-9,]+\.\d{2})`)

	// Federal tax patterns - handles FIT, FWT, FITW, Federal WH, etc.
	fedTaxPattern = regexp.MustCompile(`(?i)(?:federal\s*(?:income\s*)?(?:tax|withholding)|fed(?:eral)?\s*(?:income\s*)?(?:tax|withholding)|fed(?:eral)?\s*w/?h|fitw?|fwt)[\s:]*\$?([0-9,]+\.\d{2})`)

	// YTD simple patterns: "YTD Gross: $X" or "Year-to-Date Gross: $X"
	ytdGrossSimplePattern  = regexp.MustCompile(`(?i)(?:ytd|year[\s-]*to[\s-]*date)\s*(?:gross|total\s*earnings|earnings)[\s:]*\$?([0-9,]+\.\d{2})`)
	ytdGrossReversePattern = regexp.MustCompile(`(?i)(?:gross|total\s*earnings)\s*(?:ytd|year[\s-]*to[\s-]*date)[\s:]*\$?([0-9,]+\.\d{2})`)
	ytdFedTaxSimplePattern = regexp.MustCompile(`(?i)(?:ytd|year[\s-]*to[\s-]*date)\s*(?:federal\s*)?(?:tax|withholding)[\s:]*\$?([0-9,]+\.\d{2})`)

	// YTD table patterns: label + current + ytd on same line.
	// Use non-greedy [^\n]*? to avoid capturing partial dollar amounts.
	ytdGrossTablePattern  = regexp.MustCompile(`(?i)(?:gross\s*pay|gross\s*earnings|gross\s*wages|total\s*(?:gross|earnings)|earnings\s*total)[^\n]*?\$?([0-9,]+\.\d{2})[^\n]*?\$?([0-9,]+\.\d{2})`)
	ytdFedTaxTablePattern = regexp.MustCompile(`(?i)(?:federal\s*(?:income\s*)?(?:tax|withholding)|fed(?:eral)?\s*(?:income\s*)?(?:tax|withholding)|fed(?:eral)?\s*w/?h|fitw?|fwt)[^\n]*?\$?([0-9,]+\.\d{2})[^\n]*?\$?([0-9,]+\.\d{2})`)

	// YTD multiline table patterns: label then current then ytd on separate lines.
	ytdGrossMultilinePattern  = regexp.MustCompile(`(?i)(?:gross\s*pay|gross\s*earnings|gross\s*wages|total\s*(?:gross|earnings)|earnings\s*total)\s+\$?([0-9,]+\.\d{2})\s+\$?([0-9,]+\.\d{2})`)
	ytdFedTaxMultilinePattern = regexp.MustCompile(`(?i)(?:federal\s*(?:income\s*)?(?:tax|withholding)|fed(?:eral)?\s*(?:income\s*)?(?:tax|withholding)|fed(?:eral)?\s*w/?h|fitw?|fwt)\s+\$?([0-9,]+\.\d{2})\s+\$?([0-9,]+\.\d{2})`)

	// Earnings total pattern: "Total:" followed by 4 values (hrs, current$, ytd_hrs, ytd$).
	// Used to extract YTD gross from columnar paystubs where the earnings section
	// has a total row with hours and dollar columns.
	earningsTotalPattern = regexp.MustCompile(`(?i)total\s*:?\s+\$?([0-9,]+\.\d+)\s+\$?([0-9,]+\.\d+)\s+\$?([0-9,]+\.\d+)\s+\$?([0-9,]+\.\d+)`)

	// Hours worked pattern: "Hours Worked: 80.00" or "Total Hours: 80.00".
	hoursWorkedPattern = regexp.MustCompile(`(?i)(?:hours\s*worked|total\s*hours)[\s:]*([0-9,]+(?:\.\d+)?)`)

	// Name pattern - handles various payroll provider labels.
	// Order matters: "employee\s*name" must come before "employee" to avoid capturing "Name" as first name.
	namePattern = regexp.MustCompile(`(?i)(?:statement\s*of\s*earnings\s*for|employee\s*name|employee|name|paid?\s*to(?:\s*the\s*order\s*of)?)[\s:]*([A-Za-z]+)`)

	// Pre-tax deduction patterns.
	deduction401kPattern         = regexp.MustCompile(`(?i)(?:401\s*\(?k\)?|401k)[\s:]*\$?([0-9,]+\.\d{2})`)
	deduction403bPattern         = regexp.MustCompile(`(?i)403\s*\(?b\)?[\s:]*\$?([0-9,]+\.\d{2})`)
	deductionHSAPattern          = regexp.MustCompile(`(?i)(?:h\.?s\.?a\.?|health\s*savings)[\s:]*\$?([0-9,]+\.\d{2})`)
	deductionFSAHealthPattern    = regexp.MustCompile(`(?i)(?:(?:health(?:care)?\s*)?f\.?s\.?a\.?|flex(?:ible)?\s*spending)[\s:]*\$?([0-9,]+\.\d{2})`)
	deductionFSADependentPattern = regexp.MustCompile(`(?i)(?:dep(?:endent)?\s*(?:care\s*)?f\.?s\.?a\.?|dependent\s*care)[\s:]*\$?([0-9,]+\.\d{2})`)
	deductionCommuterPattern     = regexp.MustCompile(`(?i)(?:commuter|transit|parking\s*benefit)[\s:]*\$?([0-9,]+\.\d{2})`)

	// YTD deduction patterns — single-line (label + current + ytd on same line).
	ytdDeduction401kPattern = regexp.MustCompile(`(?i)(?:401\s*\(?k\)?|401k)[^\n]*\$?[0-9,]+\.\d{2}[^\n]*\$?([0-9,]+\.\d{2})`)
	ytdDeduction403bPattern = regexp.MustCompile(`(?i)403\s*\(?b\)?[^\n]*\$?[0-9,]+\.\d{2}[^\n]*\$?([0-9,]+\.\d{2})`)
	ytdDeductionHSAPattern  = regexp.MustCompile(`(?i)(?:h\.?s\.?a\.?|health\s*savings)[^\n]*\$?[0-9,]+\.\d{2}[^\n]*\$?([0-9,]+\.\d{2})`)

	// YTD deduction patterns — multiline (values on separate lines after label).
	ytdDeduction401kMultilinePattern = regexp.MustCompile(`(?i)(?:401\s*\(?k\)?|401k)[\s:]*\$?[0-9,]+\.\d{2}\s+\$?([0-9,]+\.\d{2})`)
	ytdDeduction403bMultilinePattern = regexp.MustCompile(`(?i)403\s*\(?b\)?[\s:]*\$?[0-9,]+\.\d{2}\s+\$?([0-9,]+\.\d{2})`)
	ytdDeductionHSAMultilinePattern  = regexp.MustCompile(`(?i)(?:h\.?s\.?a\.?|health\s*savings)[\s:]*\$?[0-9,]+\.\d{2}\s+\$?([0-9,]+\.\d{2})`)
)

func extractFields(text string) (*PaystubData, error) {
	data := &PaystubData{}
	var missing []string

	// Extract first name.
	if m := namePattern.FindStringSubmatch(text); len(m) > 1 {
		data.FirstName = m[1]
	} else {
		missing = append(missing, "employee name")
	}

	// Extract gross pay.
	if m := grossPayPattern.FindStringSubmatch(text); len(m) > 1 {
		v, err := parseAmount(m[1])
		if err == nil {
			data.GrossPay = v
		}
	} else {
		missing = append(missing, "gross pay")
	}

	// Extract federal tax withheld.
	if m := fedTaxPattern.FindStringSubmatch(text); len(m) > 1 {
		v, err := parseAmount(m[1])
		if err == nil {
			data.FederalTaxWithheld = v
		}
	} else {
		missing = append(missing, "federal tax withheld")
	}

	// Extract pay period dates (cascade through patterns).
	if !extractPayPeriodDates(text, data) {
		missing = append(missing, "pay period dates")
	}

	// Extract YTD totals (optional — not a hard failure).
	extractYTDTotals(text, data)

	// Extract pre-tax deductions (optional — not a hard failure).
	data.Deductions = extractDeductions(text)

	if len(missing) > 0 {
		return nil, &ParseError{
			Message: fmt.Sprintf("could not extract required fields: %s", strings.Join(missing, ", ")),
		}
	}

	return data, nil
}

// extractPayPeriodDates tries multiple patterns to find pay period start/end dates.
// Returns true if dates were successfully extracted.
func extractPayPeriodDates(text string, data *PaystubData) bool {
	// 1. Range pattern: "Pay Period: 01/01/2025 - 01/15/2025"
	if m := payPeriodPattern.FindStringSubmatch(text); len(m) > 2 {
		start, err1 := parseDate(m[1])
		end, err2 := parseDate(m[2])
		if err1 == nil && err2 == nil {
			data.PayPeriodStart = start
			data.PayPeriodEnd = end
			return true
		}
	}

	// 2. Separate Period Begin + Period End with adjacent dates.
	if begin := periodBeginPattern.FindStringSubmatch(text); len(begin) > 1 {
		if end := periodEndPattern.FindStringSubmatch(text); len(end) > 1 {
			startDate, err1 := parseDate(begin[1])
			endDate, err2 := parseDate(end[1])
			if err1 == nil && err2 == nil {
				data.PayPeriodStart = startDate
				data.PayPeriodEnd = endDate
				return true
			}
		}
	}

	// 3. Period Ending only (single date, infer start as end - 14 days).
	if m := periodEndPattern.FindStringSubmatch(text); len(m) > 1 {
		endDate, err := parseDate(m[1])
		if err == nil {
			data.PayPeriodEnd = endDate
			data.PayPeriodStart = endDate.AddDate(0, 0, -14)
			return true
		}
	}

	// 4. Columnar layout: Period Begin/End labels exist but dates aren't adjacent.
	// Find consecutive dates in the document and use the first two.
	if periodBeginLabelPattern.MatchString(text) || periodEndLabelPattern.MatchString(text) {
		matches := anyDatePattern.FindAllString(text, -1)
		if len(matches) >= 2 {
			var parsedDates []time.Time
			for _, d := range matches {
				if t, err := parseDate(d); err == nil {
					parsedDates = append(parsedDates, t)
					if len(parsedDates) == 2 {
						break
					}
				}
			}
			if len(parsedDates) == 2 && parsedDates[0].Before(parsedDates[1]) {
				data.PayPeriodStart = parsedDates[0]
				data.PayPeriodEnd = parsedDates[1]
				return true
			}
		}
	}

	// 5. Pay Date / Check Date (last resort, infer period as date - 14 days).
	if m := payDatePattern.FindStringSubmatch(text); len(m) > 1 {
		payDate, err := parseDate(m[1])
		if err == nil {
			data.PayPeriodEnd = payDate
			data.PayPeriodStart = payDate.AddDate(0, 0, -14)
			return true
		}
	}

	return false
}

// extractYTDTotals tries multiple patterns to find YTD gross and federal tax totals.
func extractYTDTotals(text string, data *PaystubData) {
	// YTD Gross: simple → reverse → table (single line) → earnings total (multiline).
	if m := ytdGrossSimplePattern.FindStringSubmatch(text); len(m) > 1 {
		if v, err := parseAmount(m[1]); err == nil {
			data.YTDGrossPay = v
		}
	} else if m := ytdGrossReversePattern.FindStringSubmatch(text); len(m) > 1 {
		if v, err := parseAmount(m[1]); err == nil {
			data.YTDGrossPay = v
		}
	} else if m := ytdGrossTablePattern.FindStringSubmatch(text); len(m) > 2 {
		if v, err := parseAmount(m[2]); err == nil {
			data.YTDGrossPay = v
		}
	} else if m := ytdGrossMultilinePattern.FindStringSubmatch(text); len(m) > 2 {
		if v, err := parseAmount(m[2]); err == nil {
			data.YTDGrossPay = v
		}
	} else if data.GrossPay > 0 {
		// Try earnings total pattern: "Total: hours current_$ ytd_hours ytd_$"
		// Verify the current$ matches our extracted gross pay.
		if m := earningsTotalPattern.FindStringSubmatch(text); len(m) > 4 {
			if current, err := parseAmount(m[2]); err == nil {
				if current == data.GrossPay {
					if ytd, err := parseAmount(m[4]); err == nil {
						data.YTDGrossPay = ytd
					}
					// Extract hours from the same match.
					if hrs, err := parseAmount(m[1]); err == nil {
						data.Hours = hrs
					}
					if ytdHrs, err := parseAmount(m[3]); err == nil {
						data.YTDHours = ytdHrs
					}
				}
			}
		}
	}

	// Fallback hours extraction from "Hours Worked: X" labels.
	if data.Hours == 0 {
		if m := hoursWorkedPattern.FindStringSubmatch(text); len(m) > 1 {
			if hrs, err := parseAmount(m[1]); err == nil {
				data.Hours = hrs
			}
		}
	}

	// YTD Federal Tax: simple → table (single line) → multiline table.
	if m := ytdFedTaxSimplePattern.FindStringSubmatch(text); len(m) > 1 {
		if v, err := parseAmount(m[1]); err == nil {
			data.YTDFederalTaxWithheld = v
		}
	} else if m := ytdFedTaxTablePattern.FindStringSubmatch(text); len(m) > 2 {
		if v, err := parseAmount(m[2]); err == nil {
			data.YTDFederalTaxWithheld = v
		}
	} else if m := ytdFedTaxMultilinePattern.FindStringSubmatch(text); len(m) > 2 {
		if v, err := parseAmount(m[2]); err == nil {
			data.YTDFederalTaxWithheld = v
		}
	}
}

func extractDeductions(text string) []Deduction {
	type deductionSpec struct {
		typ              string
		pattern          *regexp.Regexp
		ytdPattern       *regexp.Regexp // single-line YTD pattern
		ytdMultiline     *regexp.Regexp // multiline YTD fallback
	}

	specs := []deductionSpec{
		{"401k", deduction401kPattern, ytdDeduction401kPattern, ytdDeduction401kMultilinePattern},
		{"403b", deduction403bPattern, ytdDeduction403bPattern, ytdDeduction403bMultilinePattern},
		{"hsa", deductionHSAPattern, ytdDeductionHSAPattern, ytdDeductionHSAMultilinePattern},
		{"fsa_dependent", deductionFSADependentPattern, nil, nil},
		{"fsa_health", deductionFSAHealthPattern, nil, nil},
		{"commuter", deductionCommuterPattern, nil, nil},
	}

	var deductions []Deduction
	for _, spec := range specs {
		if m := spec.pattern.FindStringSubmatch(text); len(m) > 1 {
			amount, err := parseAmount(m[1])
			if err != nil || amount == 0 {
				continue
			}
			d := Deduction{Type: spec.typ, Amount: amount}
			// Try single-line YTD pattern first, then multiline fallback.
			if spec.ytdPattern != nil {
				if ym := spec.ytdPattern.FindStringSubmatch(text); len(ym) > 1 {
					if ytd, err := parseAmount(ym[1]); err == nil {
						d.YTDAmount = ytd
					}
				} else if spec.ytdMultiline != nil {
					if ym := spec.ytdMultiline.FindStringSubmatch(text); len(ym) > 1 {
						if ytd, err := parseAmount(ym[1]); err == nil {
							d.YTDAmount = ytd
						}
					}
				}
			}
			deductions = append(deductions, d)
		}
	}
	return deductions
}

func parseAmount(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	return strconv.ParseFloat(s, 64)
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, fmt := range dateFormats {
		if t, err := time.Parse(fmt, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format: %s", s)
}
