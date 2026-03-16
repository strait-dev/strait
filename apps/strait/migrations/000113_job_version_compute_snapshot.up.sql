ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS machine_preset TEXT NOT NULL DEFAULT '';
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS image_uri TEXT NOT NULL DEFAULT '';
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS region TEXT NOT NULL DEFAULT '';

UPDATE job_versions jv SET
    machine_preset = COALESCE(j.machine_preset, ''),
    image_uri = COALESCE(j.image_uri, ''),
    region = COALESCE(j.region, '')
FROM jobs j WHERE jv.job_id = j.id
AND jv.machine_preset = '';
