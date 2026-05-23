-- No-op. VALIDATE CONSTRAINT only flips a catalog flag; there is no "un-validate"
-- and leaving the constraints valid on the way down is harmless. The constraints
-- themselves are dropped by 000288's down migration.
SELECT 1;
