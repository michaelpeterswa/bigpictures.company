create extension if not exists postgis;

create table panoramas (
    id            uuid primary key default gen_random_uuid(),
    slug          text unique not null,
    title         text not null,
    description   text,
    captured_at   timestamptz,
    location      geography(point, 4326),
    width         integer not null,
    height        integer not null,
    tile_path     text not null,
    thumb_prefix  text not null,
    original_path text not null,
    exif          jsonb,
    tags          text[] not null default '{}',
    created_at    timestamptz not null default now()
);

create index panoramas_location_gix    on panoramas using gist (location);
create index panoramas_captured_at_idx on panoramas (captured_at desc);
create index panoramas_tags_gin        on panoramas using gin (tags);
