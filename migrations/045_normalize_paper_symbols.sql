-- Merge paper positions that differ only by symbol/exchange casing, then normalize keys.

WITH canon AS (
  SELECT
    (array_agg(id ORDER BY created_at, id::text))[1] AS keep_id,
    array_agg(id) AS all_ids,
    paper_account_id,
    UPPER(TRIM(exchange)) AS exchange,
    UPPER(TRIM(symbol)) AS symbol,
    product,
    SUM(quantity)::double precision AS quantity,
    CASE
      WHEN SUM(ABS(quantity)) = 0 THEN 0
      ELSE SUM(ABS(quantity) * avg_entry_price) / SUM(ABS(quantity))
    END AS avg_entry_price,
    (array_agg(last_price ORDER BY updated_at DESC))[1]::double precision AS last_price
  FROM paper_positions
  GROUP BY paper_account_id, UPPER(TRIM(exchange)), UPPER(TRIM(symbol)), product
  HAVING COUNT(*) > 1
),
updated AS (
  UPDATE paper_positions p
  SET
    exchange = c.exchange,
    symbol = c.symbol,
    quantity = c.quantity,
    avg_entry_price = c.avg_entry_price,
    last_price = c.last_price,
    unrealized_pnl = CASE
      WHEN c.quantity > 0 THEN (c.last_price - c.avg_entry_price) * c.quantity
      WHEN c.quantity < 0 THEN (c.avg_entry_price - c.last_price) * (-c.quantity)
      ELSE 0
    END,
    updated_at = NOW()
  FROM canon c
  WHERE p.id = c.keep_id
  RETURNING c.all_ids, c.keep_id
)
DELETE FROM paper_positions p
USING updated u
WHERE p.id = ANY(u.all_ids) AND p.id <> u.keep_id;

UPDATE paper_positions
SET symbol = UPPER(TRIM(symbol)), exchange = UPPER(TRIM(exchange))
WHERE symbol <> UPPER(TRIM(symbol)) OR exchange <> UPPER(TRIM(exchange));

UPDATE paper_orders
SET symbol = UPPER(TRIM(symbol)), exchange = UPPER(TRIM(exchange))
WHERE symbol <> UPPER(TRIM(symbol)) OR exchange <> UPPER(TRIM(exchange));

UPDATE paper_trades
SET symbol = UPPER(TRIM(symbol)), exchange = UPPER(TRIM(exchange))
WHERE symbol <> UPPER(TRIM(symbol)) OR exchange <> UPPER(TRIM(exchange));
