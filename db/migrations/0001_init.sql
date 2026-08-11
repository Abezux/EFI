-- Migration: 0001_init.sql
-- Description: Create initial schema for V1 scope (channels and raw_posts) with idempotency constraints and least-privilege role.

-- Enable pgvector extension
create extension if not exists vector;

-- Channels table: stores tracked public Telegram channels
create table if not exists channels (
  id bigserial primary key,
  telegram_channel_id bigint not null unique,
  name text not null,
  handle text,
  category_default text,
  is_active boolean not null default true,
  trust_tier integer not null default 1,
  added_at timestamptz not null default now(),
  last_seen_at timestamptz
);

create index if not exists idx_channels_telegram_channel_id on channels(telegram_channel_id);
create index if not exists idx_channels_is_active on channels(is_active);

-- Raw posts table: immutable storage for ingested Telegram messages
create table if not exists raw_posts (
  id bigserial primary key,
  channel_id bigint not null references channels(id) on delete restrict,
  telegram_message_id bigint not null,
  raw_text text not null,
  raw_entities jsonb not null default '[]'::jsonb,
  media_refs jsonb not null default '[]'::jsonb,
  posted_at timestamptz not null,
  ingested_at timestamptz not null default now(),
  normalized_text text,
  language varchar(10),
  embedding vector(1536),
  processing_status varchar(50) not null default 'ingested',
  constraint uq_raw_posts_channel_message unique (channel_id, telegram_message_id)
);

create index if not exists idx_raw_posts_channel_id on raw_posts(channel_id);
create index if not exists idx_raw_posts_processing_status on raw_posts(processing_status);
create index if not exists idx_raw_posts_posted_at on raw_posts(posted_at desc);

-- Application DB role (least privilege)
do $$
begin
  if not exists (select from pg_roles where rolname = 'efi_app') then
    create role efi_app with login password 'efi_app_pass';
  end if;
end
$$;

-- Grant permissions to efi_app role
grant connect on database current_database() to efi_app;
grant usage on schema public to efi_app;
grant select, insert, update on table channels to efi_app;
grant select, insert, update on table raw_posts to efi_app;
grant usage, select on all sequences in schema public to efi_app;
alter default privileges in schema public grant usage, select on sequences to efi_app;
alter default privileges in schema public grant select, insert, update on tables to efi_app;
