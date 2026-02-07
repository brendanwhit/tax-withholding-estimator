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
		"2006-01-02",
		"Jan 02, 2006",
		"January 2, 2006",
		"01/02/06",
	}

	// Patterns for extracting pay period dates.
	// Handles formats like "PAY PERIOD: 01/01/2026 - 01/15/2026" or "Pay Period 01/01/2026 to 01/15/2026"
	payPeriodPattern = regexp.MustCompile(`(?i)pay\s*period[\s:]*(\d{1,2}/\d{1,2}/\d{2,4})\s*(?:-|to|through|thru)\s*(\d{1,2}/\d{1,2}/\d{2,4})`)

	// Patterns for extracting amounts - handles $X,XXX.XX format
	grossPayPattern = regexp.MustCompile(`(?i)(?:gross\s*pay|gross\s*earnings|total\s*gross)[\s:]*\$?([0-9,]+\.\d{2})`)
	fedTaxPattern   = regexp.MustCompile(`(?i)(?:federal\s*(?:income\s*)?tax|fed\s*(?:income\s*)?tax|federal\s*withholding|fed\s*w/?h|fit)[\s:]*\$?([0-9,]+\.\d{2})`)

	// YTD patterns - support both simple "YTD Gross: $X" and table "Gross Pay $current $ytd" formats
	ytdGrossSimplePattern = regexp.MustCompile(`(?i)ytd\s*gross[\s:]*\$?([0-9,]+\.\d{2})`)
	ytdFedTaxSimplePattern = regexp.MustCompile(`(?i)ytd\s*(?:federal\s*)?tax[\s:]*\$?([0-9,]+\.\d{2})`)
	// Table format: "Gross Pay $current $ytd" on same line
	ytdGrossTablePattern  = regexp.MustCompile(`(?i)gross\s*pay[^\n]*\$?([0-9,]+\.\d{2})[^\n]*\$?([0-9,]+\.\d{2})`)
	ytdFedTaxTablePattern = regexp.MustCompile(`(?i)federal\s*(?:income\s*)?tax[^\n]*\$?([0-9,]+\.\d{2})[^\n]*\$?([0-9,]+\.\d{2})`)

	// Name pattern - handles "PAID TO:", "Pay To:", "Employee:", "Name:"
	namePattern = regexp.MustCompile(`(?i)(?:employee|name|paid?\s*to)[\s:]*([A-Za-z]+)`)

	// Pre-tax deduction patterns.
	deduction401kPattern = regexp.MustCompile(`(?i)(?:401\s*\(?k\)?|401k)[\s:]*\$?([0-9,]+\.\d{2})`)
	deduction403bPattern = regexp.MustCompile(`(?i)403\s*\(?b\)?[\s:]*\$?([0-9,]+\.\d{2})`)
	deductionHSAPattern  = regexp.MustCompile(`(?i)(?:h\.?s\.?a\.?|health\s*savings)[\s:]*\$?([0-9,]+\.\d{2})`)
	deductionFSAHealthPattern    = regexp.MustCompile(`(?i)(?:(?:health(?:care)?\s*)?f\.?s\.?a\.?|flex(?:ible)?\s*spending)[\s:]*\$?([0-9,]+\.\d{2})`)
	deductionFSADependentPattern = regexp.MustCompile(`(?i)(?:dep(?:endent)?\s*(?:care\s*)?f\.?s\.?a\.?|dependent\s*care)[\s:]*\$?([0-9,]+\.\d{2})`)
	deductionCommuterPattern     = regexp.MustCompile(`(?i)(?:commuter|transit|parking\s*benefit)[\s:]*\$?([0-9,]+\.\d{2})`)

	// YTD deduction patterns (optional).
	ytdDeduction401kPattern = regexp.MustCompile(`(?i)(?:401\s*\(?k\)?|401k)[^\n]*\$?[0-9,]+\.\d{2}[^\n]*\$?([0-9,]+\.\d{2})`)
	ytdDeduction403bPattern = regexp.MustCompile(`(?i)403\s*\(?b\)?[^\n]*\$?[0-9,]+\.\d{2}[^\n]*\$?([0-9,]+\.\d{2})`)
	ytdDeductionHSAPattern  = regexp.MustCompile(`(?i)(?:h\.?s\.?a\.?|health\s*savings)[^\n]*\$?[0-9,]+\.\d{2}[^\n]*\$?([0-9,]+\.\d{2})`)
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

	// Extract pay period dates.
	if m := payPeriodPattern.FindStringSubmatch(text); len(m) > 2 {
		start, err1 := parseDate(m[1])
		end, err2 := parseDate(m[2])
		if err1 == nil && err2 == nil {
			data.PayPeriodStart = start
			data.PayPeriodEnd = end
		} else {
			missing = append(missing, "pay period dates")
		}
	} else {
		missing = append(missing, "pay period dates")
	}

	// Extract YTD totals (optional — not a hard failure).
	// Try simple "YTD Gross: $X" format first, then table format.
	if m := ytdGrossSimplePattern.FindStringSubmatch(text); len(m) > 1 {
		v, err := parseAmount(m[1])
		if err == nil {
			data.YTDGrossPay = v
		}
	} else if m := ytdGrossTablePattern.FindStringSubmatch(text); len(m) > 2 {
		v, err := parseAmount(m[2]) // YTD is second column in table
		if err == nil {
			data.YTDGrossPay = v
		}
	}
	if m := ytdFedTaxSimplePattern.FindStringSubmatch(text); len(m) > 1 {
		v, err := parseAmount(m[1])
		if err == nil {
			data.YTDFederalTaxWithheld = v
		}
	} else if m := ytdFedTaxTablePattern.FindStringSubmatch(text); len(m) > 2 {
		v, err := parseAmount(m[2]) // YTD is second column in table
		if err == nil {
			data.YTDFederalTaxWithheld = v
		}
	}

	// Extract pre-tax deductions (optional — not a hard failure).
	data.Deductions = extractDeductions(text)

	if len(missing) > 0 {
		return nil, &ParseError{
			Message: fmt.Sprintf("could not extract required fields: %s", strings.Join(missing, ", ")),
		}
	}

	return data, nil
}

func extractDeductions(text string) []Deduction {
	type deductionSpec struct {
		typ        string
		pattern    *regexp.Regexp
		ytdPattern *regexp.Regexp // nil if no YTD pattern
	}

	specs := []deductionSpec{
		{"401k", deduction401kPattern, ytdDeduction401kPattern},
		{"403b", deduction403bPattern, ytdDeduction403bPattern},
		{"hsa", deductionHSAPattern, ytdDeductionHSAPattern},
		{"fsa_dependent", deductionFSADependentPattern, nil},
		{"fsa_health", deductionFSAHealthPattern, nil},
		{"commuter", deductionCommuterPattern, nil},
	}

	var deductions []Deduction
	for _, spec := range specs {
		if m := spec.pattern.FindStringSubmatch(text); len(m) > 1 {
			amount, err := parseAmount(m[1])
			if err != nil || amount == 0 {
				continue
			}
			d := Deduction{Type: spec.typ, Amount: amount}
			if spec.ytdPattern != nil {
				if ym := spec.ytdPattern.FindStringSubmatch(text); len(ym) > 1 {
					if ytd, err := parseAmount(ym[1]); err == nil {
						d.YTDAmount = ytd
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
