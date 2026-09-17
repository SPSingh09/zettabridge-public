-- Four subscription tiers: free, paper, pro, pro_plus.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_plan_check;

UPDATE users SET plan = 'free' WHERE plan NOT IN ('free', 'paper', 'pro', 'pro_plus');

ALTER TABLE users ADD CONSTRAINT users_plan_check CHECK (plan IN ('free', 'paper', 'pro', 'pro_plus'));
