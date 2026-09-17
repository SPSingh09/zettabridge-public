-- Migration 039 seeded NIFTY/BANKNIFTY (instrument_type INDEX, under
-- indian_equity) with a placeholder lot_size of 1, same as the individual
-- cash-equity symbols in that same seed. That's wrong for these two: unlike
-- single-share cash equities, NSE index derivatives are only ever traded in
-- the exchange-specified lot, never quantity 1.
--
-- Per NSE's contract-value-band revision effective January 2026:
--   NIFTY 50 lot size:   65
--   NIFTY Bank (BANKNIFTY) lot size: 30
-- (Both previously smaller — NSE/SEBI revise these periodically to keep
-- contract notional value within a mandated band, most recently ~Jan 2026.
-- When NSE issues the next revision, update via the admin instruments UI/API
-- — this migration only fixes today's known-stale default, it isn't meant to
-- be a source of truth going forward.)

UPDATE instruments SET lot_size = 65 WHERE market_profile_code = 'indian_equity' AND symbol = 'NIFTY';
UPDATE instruments SET lot_size = 30 WHERE market_profile_code = 'indian_equity' AND symbol = 'BANKNIFTY';
