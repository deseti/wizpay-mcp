ALTER TABLE identities
    ADD COLUMN provider_subject text;

UPDATE identities
SET provider_subject = 'legacy-unmapped:' || user_id
WHERE provider_subject IS NULL;

ALTER TABLE identities
    ALTER COLUMN provider_subject SET NOT NULL,
    ADD CONSTRAINT identities_provider_subject_nonempty CHECK (provider_subject <> ''),
    ADD CONSTRAINT identities_provider_subject_unique UNIQUE (tenant_id, provider, provider_subject);
