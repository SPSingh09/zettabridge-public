-- Store resolved product (MIS/CNC/NRML) on each trade audit row for dashboard history.
ALTER TABLE trades ADD COLUMN IF NOT EXISTS product TEXT NOT NULL DEFAULT '';
