INSERT INTO users (id, email, password_hash, plan, role, status, email_verified_at, created_at)
VALUES (
  gen_random_uuid()::text,
  :'admin_email',
  crypt(:'admin_password', gen_salt('bf')),
  'free',
  'admin',
  'active',
  NOW(),
  NOW()
)
ON CONFLICT (email) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    plan = 'free',
    role = 'admin',
    status = 'active',
    email_verified_at = COALESCE(users.email_verified_at, NOW());
