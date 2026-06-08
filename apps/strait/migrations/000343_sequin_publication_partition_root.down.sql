DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_publication WHERE pubname = 'sequin_strait_pub') THEN
        ALTER PUBLICATION sequin_strait_pub SET (publish_via_partition_root = false);
    END IF;
END
$$;
