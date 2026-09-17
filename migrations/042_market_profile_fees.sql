-- Admin-configurable fee/slippage modeling for Paper Trading. Simplified vs.
-- a fully accurate Indian brokerage/tax stack (STT differs by equity-delivery
-- vs. intraday vs. F&O; GST on brokerage; stamp duty; exchange transaction
-- charges; SEBI turnover fees) -- one combined fee_bps standing in for
-- brokerage+taxes+charges together, plus slippage_bps applied to fill price.
-- Default 0 on both columns means zero behavior change for every existing
-- profile until an admin opts in.

ALTER TABLE market_profiles
    ADD COLUMN slippage_bps NUMERIC(8,4) NOT NULL DEFAULT 0,
    ADD COLUMN fee_bps      NUMERIC(8,4) NOT NULL DEFAULT 0;
