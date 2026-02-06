# Design Decisions

## SQLite Driver: modernc.org/sqlite (pure Go)
Chose the pure-Go SQLite driver over `mattn/go-sqlite3` to avoid requiring a C compiler (gcc) in the build environment. This simplifies deployment and CI. The driver is API-compatible via `database/sql`.

## Template Architecture: header/footer blocks
Used separate `{{define "header"}}` and `{{define "footer"}}` blocks instead of a single `{{define "layout"}}` wrapper template. This avoids the Go template limitation where multiple files defining the same `{{define "content"}}` block would overwrite each other when parsed with `ParseGlob`.

## Paystub Deduplication: upsert on (person, period_start, period_end)
Duplicate paystub uploads are detected by the combination of person name, pay period start, and pay period end dates. On conflict, the record is updated with the latest values rather than rejected. This allows users to re-upload corrected paystubs.

## Tax Bracket Data: hardcoded fallback
Tax brackets for 2025 and 2026 are hardcoded as fallback data. The system first checks the SQLite cache, then falls back to hardcoded values and caches them. This ensures the app works offline and without IRS data fetch infrastructure.

## Standard Deduction Values
Used IRS Revenue Procedure 2024-40 for 2025 values. 2026 values are projected estimates based on typical inflation adjustments.

## Pay Period Estimation: default 26 (biweekly)
When the number of pay periods per year is not specified, defaults to 26 (biweekly). This is the most common pay frequency in the US.

## Withholding Recommendation: applied to higher earner
The additional withholding recommendation is applied to the higher earner's paycheck, as this is the most practical approach for a married filing jointly scenario where one spouse earns more.

## PII Handling: regex-based stripping
Sensitive data (SSNs, EINs, bank account numbers) is stripped using regex patterns before any field extraction. Only the first name is retained as an identifier. No full names, addresses, or financial identifiers are stored.
