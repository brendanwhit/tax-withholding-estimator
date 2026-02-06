-- Initial schema for tax withholding estimator.

CREATE TABLE IF NOT EXISTS tax_brackets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tax_year INTEGER NOT NULL,
    filing_status TEXT NOT NULL,
    bracket_min REAL NOT NULL,
    bracket_max REAL,  -- NULL means no upper limit
    rate REAL NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tax_year, filing_status, bracket_min)
);

CREATE TABLE IF NOT EXISTS standard_deductions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tax_year INTEGER NOT NULL,
    filing_status TEXT NOT NULL,
    amount REAL NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tax_year, filing_status)
);

CREATE TABLE IF NOT EXISTS filing_status_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tax_year INTEGER NOT NULL UNIQUE,
    filing_status TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS paystubs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    person_name TEXT NOT NULL,
    tax_year INTEGER NOT NULL,
    pay_period_start DATE NOT NULL,
    pay_period_end DATE NOT NULL,
    gross_pay REAL NOT NULL,
    federal_tax_withheld REAL NOT NULL,
    ytd_gross_pay REAL,
    ytd_federal_tax_withheld REAL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(person_name, pay_period_start, pay_period_end)
);

CREATE TABLE IF NOT EXISTS bracket_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tax_year INTEGER NOT NULL UNIQUE,
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    source TEXT NOT NULL
);
