-- users
create table if not exists users (
                                     id uuid primary key,

                                     email text not null,
                                     display_name text not null,
                                     password_hash text not null,
                                     roles text[] not null default array['user']::text[],

                                     email_verified_at timestamptz null,
                                     disabled_at timestamptz null,
                                     password_changed_at timestamptz not null default now(),

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    constraint users_email_lower_chk check (email = lower(email))
    );

create unique index if not exists users_email_uq on users (email);

-- refresh_sessions
create table if not exists refresh_sessions (
                                                id uuid primary key,

                                                user_id uuid not null references users(id) on delete cascade,

    token_hash bytea not null unique,

    device_id text null,
    device_name text null,

    created_at timestamptz not null default now(),
    expires_at timestamptz not null,

    last_used_at timestamptz null,

    revoked_at timestamptz null,
    replaced_by uuid null references refresh_sessions(id),

    ip inet null,
    user_agent text null
    );

create index if not exists refresh_sessions_user_active_idx
    on refresh_sessions (user_id, created_at desc)
    where revoked_at is null;

create index if not exists refresh_sessions_expires_idx
    on refresh_sessions (expires_at);

