-- Migration: 0006_api_read_role.sql
-- Description: Create least-privilege read-only role for the public API service.

-- Create efi_api role idempotently if it does not already exist
do $$
begin
  if not exists (select from pg_roles where rolname = 'efi_api') then
    create role efi_api with login password 'efi_api_pass';
  end if;
end
$$;

-- Grant connect on current database
do $$
begin
  execute format('grant connect on database %I to efi_api', current_database());
end
$$;

-- Grant schema usage
grant usage on schema public to efi_api;

-- Grant SELECT ONLY on public-facing tables
grant select on table news_events, event_sources, raw_posts, channels, categories, entities, event_entities to efi_api;

-- processing_audit is an internal operational/audit table and is explicitly excluded from efi_api grants.
