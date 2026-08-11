-- Migration: 0002_seed_channels.sql
-- Description: Seed initial public Ethiopian Telegram channels for V1 ingestion.

insert into channels (telegram_channel_id, name, handle, category_default, is_active, trust_tier, added_at, last_seen_at)
values
  (1001111111111, 'Ethiopian News Agency', 'ena_news', 'general', true, 1, now(), now()),
  (1001222222222, 'Fana Broadcasting Corporate', 'fana_broadcast', 'general', true, 1, now(), now()),
  (1001333333333, 'Addis Standard', 'addis_standard', 'politics', true, 1, now(), now()),
  (1001444444444, 'Capital Ethiopia', 'capital_ethiopia', 'business', true, 1, now(), now()),
  (1001555555555, 'Tikvah Ethiopia', 'tikvahethiopia', 'breaking', true, 2, now(), now()),
  (1001666666666, 'Walta Information Center', 'waltainfo', 'general', true, 1, now(), now()),
  (1001777777777, 'Sheger FM 102.1', 'shegerfm', 'local', true, 2, now(), now())
on conflict (telegram_channel_id) do update
set
  name = excluded.name,
  handle = excluded.handle,
  category_default = excluded.category_default,
  is_active = excluded.is_active,
  trust_tier = excluded.trust_tier,
  last_seen_at = now();
