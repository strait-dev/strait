DROP INDEX IF EXISTS idx_referrals_referred_email_activated;
ALTER TABLE referrals DROP COLUMN IF EXISTS expires_at;
ALTER TABLE referrals DROP COLUMN IF EXISTS referred_email;
