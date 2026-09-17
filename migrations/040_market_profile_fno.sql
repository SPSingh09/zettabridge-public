-- Phase 7 follow-up: seed the Indian F&O market profile. Market profiles are
-- curated/pre-configured (added via migration as needed, not created ad hoc
-- by admins through the API — see the removed AdminCreateMarketProfile) --
-- this is the second one, alongside indian_equity from migration 039.
-- Same NSE session hours as equity; admins add F&O instruments (futures/
-- options symbols, lot sizes) via the existing instruments UI/API once this
-- profile exists.

INSERT INTO market_profiles (code, name, base_currency, timezone, trading_days, active) VALUES (
    'indian_fno',
    'Indian F&O',
    'INR',
    'Asia/Kolkata',
    '{
        "mon": [{"start": "09:15", "end": "15:30"}],
        "tue": [{"start": "09:15", "end": "15:30"}],
        "wed": [{"start": "09:15", "end": "15:30"}],
        "thu": [{"start": "09:15", "end": "15:30"}],
        "fri": [{"start": "09:15", "end": "15:30"}]
    }'::jsonb,
    true
);
