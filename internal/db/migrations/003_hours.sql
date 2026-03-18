-- Add hours columns to paystubs for hourly worker support.

ALTER TABLE paystubs ADD COLUMN hours REAL;
ALTER TABLE paystubs ADD COLUMN ytd_hours REAL;
