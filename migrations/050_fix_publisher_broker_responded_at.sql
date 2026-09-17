-- Migration 048 backfilled broker_responded_at = updated_at for every
-- pre-existing trade row. For Kite Publisher trades, updated_at was bumped
-- by UpdatePublisherTradeStatus at human-confirmation time (the user
-- approving a basket inside Kite, which can take minutes) — not at genuine
-- broker-submission time. That backfill re-introduced the exact
-- confirmation-delay contamination broker_responded_at was created to avoid,
-- inflating the Avg Latency metric on historical webhooks with Publisher
-- trades. Publisher trades never have broker_responded_at set going forward
-- (UpdatePublisherTradeStatus intentionally leaves it untouched) — null it
-- back out here for the rows the backfill incorrectly populated.
UPDATE trades t
SET broker_responded_at = NULL
FROM webhooks w
JOIN broker_credentials bc ON bc.id = w.broker_cred_id
WHERE t.webhook_id = w.id
  AND bc.execution_mode = 'publisher'
  AND t.broker_responded_at IS NOT NULL;
