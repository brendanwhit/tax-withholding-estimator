# Tax Withholding Estimator

A personal Go web app for estimating federal tax withholding across two incomes, fed by periodic paystub PDF uploads. Built with Go + htmx + Go templates and SQLite for persistence.

## Features

- Upload paystub PDFs to extract gross pay, federal tax withheld, and pay period dates
- Automatic PII stripping — only first names are stored (no SSNs, addresses, or bank info)
- Combined withholding calculation for two earners (e.g. married filing jointly)
- Recommends additional per-paycheck withholding for the higher earner
- Browse 2025/2026 federal tax brackets by filing status
- Supports Single, Married Filing Jointly, Married Filing Separately, and Head of Household

## Prerequisites

- Go 1.22+ (no C compiler needed — uses a pure-Go SQLite driver)

## Quick Start

```bash
# Clone the repo
git clone https://github.com/brendanwhit/tax-withholding-estimator.git
cd tax-withholding-estimator

# Run the server
go run ./cmd/server
```

Open http://localhost:8080 in your browser.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DB_PATH` | `./data/tax.db` | Path to the SQLite database file |
| `ADDR` | `:8080` | Address and port to listen on |
| `TEMPLATE_DIR` | `templates` | Path to the HTML template directory |

Example:

```bash
DB_PATH=/home/pi/tax.db ADDR=:3000 go run ./cmd/server
```

## Usage

### 1. Upload Paystubs

Navigate to `/upload` and upload a PDF of your paystub. The parser extracts:

- First name (used as identifier)
- Gross pay for the period
- Federal income tax withheld
- Pay period start and end dates
- YTD gross pay and YTD federal tax withheld (if present)

Upload a paystub each pay period for both earners to refine the estimate over time. Duplicate uploads (same person, same pay period) update the existing record.

### 2. Set Filing Status

On the dashboard (`/`), select your filing status for the tax year. This determines which tax brackets and standard deduction are used.

### 3. View Withholding Recommendation

The dashboard shows:

- Estimated annual income (projected from uploaded pay periods)
- Total federal tax liability based on the applicable brackets
- Total withheld to date across both earners
- Remaining tax owed and recommended additional withholding per paycheck

### 4. Explore Tax Brackets

Visit `/brackets` to browse federal tax brackets by year (2025/2026) and filing status, with a color-coded visualization of the marginal rates.

## Development

```bash
# Run in dev mode (enables /admin/clear-db endpoint)
make dev

# Run in dev mode with live reload (rebuilds on file changes)
make dev-watch

# Run tests
make test

# Run linter
make lint

# Build
make build

# Run all checks (lint + test + build)
make check
```

### Dev-Only Features

When running with `make dev` (which uses `-tags dev`), the following features are available:

- **POST /admin/clear-db** — Clears all data from the database (paystubs, filing status, cached brackets). Useful for testing with a clean slate. This endpoint does not exist in production builds.

## Project Structure

```
cmd/server/          Main entry point
internal/
  db/                SQLite store, migrations, paystub CRUD
  tax/               Tax bracket calculation, hardcoded bracket data, DB cache
  pdf/               PDF text extraction, PII stripping
  calc/              Withholding calculator engine
  handler/           HTTP handlers with htmx support
templates/           Go HTML templates (dashboard, upload, brackets)
migrations/          SQL migration files (embedded at build time)
```

## Limitations

- Federal tax only (no state tax calculations)
- Text-based PDFs only (no OCR for image-based PDFs)
- Single-user (no authentication)
- PDF parsing uses regex patterns — may need adjustment for different paystub formats
