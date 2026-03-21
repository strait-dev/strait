ALTER TABLE referrals ADD COLUMN IF NOT EXISTS referred_email TEXT;
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS idx_referrals_referred_email_activated
  ON referrals(referred_email) WHERE status = 'activated';
