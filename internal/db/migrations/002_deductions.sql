-- Pre-tax deductions extracted from paystubs.

CREATE TABLE IF NOT EXISTS pre_tax_deductions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    paystub_id INTEGER NOT NULL REFERENCES paystubs(id) ON DELETE CASCADE,
    deduction_type TEXT NOT NULL,  -- '401k', '403b', 'hsa', 'fsa_health', 'fsa_dependent', 'commuter'
    amount REAL NOT NULL,
    ytd_amount REAL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(paystub_id, deduction_type)
);

-- IRS contribution limits per year, cached like tax brackets.

CREATE TABLE IF NOT EXISTS contribution_limits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tax_year INTEGER NOT NULL,
    deduction_type TEXT NOT NULL,
    annual_limit REAL NOT NULL,
    catch_up_limit REAL,  -- additional for age 50+
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tax_year, deduction_type)
);
