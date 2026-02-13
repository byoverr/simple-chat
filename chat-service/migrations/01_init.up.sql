-- chats
create table if not exists chats (
                                     id uuid primary key,

                                     type smallint not null, -- 1 DIRECT, 2 GROUP, 3 CHANNEL
                                     title text not null default '',
                                     avatar_url text null,

                                     created_by uuid not null,
                                     created_at timestamptz not null default now(),

    members_count int not null default 0,

    -- денормализация для быстрого ListChats
    last_message_id uuid null,
    last_message_seq bigint not null default 0,
    last_message_at timestamptz null,

    -- для DIRECT: уникальность пары участников (заполняется приложением)
    direct_key text null
    );

create unique index if not exists chats_direct_key_uq
    on chats(direct_key)
    where direct_key is not null;

alter table chats
    add constraint chats_type_chk
        check (type in (1,2,3));

-- Для GROUP/CHANNEL требуем непустой title (DIRECT может быть пустым)
alter table chats
    add constraint chats_title_chk
        check (
            (type in (2,3) and length(btrim(title)) > 0)
                or (type = 1)
            );

-- DIRECT: если direct_key задан, то type должен быть DIRECT
alter table chats
    add constraint chats_direct_key_type_chk
        check (
            (direct_key is null) or (type = 1)
            );

-- chat_members
create table if not exists chat_members (
                                            chat_id uuid not null references chats(id) on delete cascade,
    user_id uuid not null,

    role text not null default 'member', -- member/admin
    joined_at timestamptz not null default now(),
    left_at timestamptz null,

    primary key (chat_id, user_id)
    );

create index if not exists chat_members_user_idx
    on chat_members(user_id, joined_at desc);

create index if not exists chat_members_chat_active_idx
    on chat_members(chat_id)
    where left_at is null;

-- messages
create table if not exists messages (
                                        id uuid primary key,

                                        chat_id uuid not null references chats(id) on delete cascade,
    sender_id uuid not null,

    client_msg_id uuid not null, -- идемпотентность (ретраи клиента)
    text text not null,

    seq bigint not null, -- порядок внутри чата
    created_at timestamptz not null default now(),
    edited_at timestamptz null,

    reply_to_message_id uuid null references messages(id) on delete set null
    );

create unique index if not exists messages_chat_client_uq
    on messages(chat_id, client_msg_id);

create index if not exists messages_chat_seq_desc_idx
    on messages(chat_id, seq desc);

-- chat_state (unread/read как в Telegram)
create table if not exists chat_state (
                                          chat_id uuid not null references chats(id) on delete cascade,
    user_id uuid not null,

    last_read_seq bigint not null default 0,
    updated_at timestamptz not null default now(),

    primary key (chat_id, user_id)
    );

create index if not exists chat_state_user_idx
    on chat_state(user_id, updated_at desc);

-- =========================================================
-- seq внутри чата (монотонный, конкурентно-безопасный)
-- =========================================================

create table if not exists chat_seq (
                                        chat_id uuid primary key references chats(id) on delete cascade,
    last_seq bigint not null default 0
    );

create or replace function chat_next_seq(p_chat_id uuid)
returns bigint
language plpgsql
as $$
declare v bigint;
begin
insert into chat_seq(chat_id, last_seq)
values (p_chat_id, 1)
    on conflict (chat_id)
  do update set last_seq = chat_seq.last_seq + 1
             returning last_seq into v;

return v;
end;
$$;

create or replace function trg_messages_set_seq()
returns trigger
language plpgsql
as $$
begin
  if new.seq is null or new.seq = 0 then
    new.seq := chat_next_seq(new.chat_id);
end if;
return new;
end;
$$;

drop trigger if exists messages_set_seq on messages;
create trigger messages_set_seq
    before insert on messages
    for each row
    execute function trg_messages_set_seq();

-- =========================================================
-- денормализация last_message_* в chats
-- =========================================================

create or replace function trg_chats_update_last_message()
returns trigger
language plpgsql
as $$
begin
update chats
set last_message_id = new.id,
    last_message_seq = new.seq,
    last_message_at = new.created_at
where id = new.chat_id
  and new.seq >= last_message_seq;

return null;
end;
$$;

drop trigger if exists chats_update_last_message on messages;
create trigger chats_update_last_message
    after insert on messages
    for each row
    execute function trg_chats_update_last_message();

-- =========================================================
-- members_count в chats
-- =========================================================

create or replace function trg_members_count_ins()
returns trigger
language plpgsql
as $$
begin
  if new.left_at is null then
update chats set members_count = members_count + 1 where id = new.chat_id;
end if;
return null;
end;
$$;

create or replace function trg_members_count_upd()
returns trigger
language plpgsql
as $$
begin
  -- ушёл из чата
  if old.left_at is null and new.left_at is not null then
update chats set members_count = greatest(members_count - 1, 0) where id = new.chat_id;
end if;

  -- вернулся (если вдруг поддержишь)
  if old.left_at is not null and new.left_at is null then
update chats set members_count = members_count + 1 where id = new.chat_id;
end if;

return null;
end;
$$;

create or replace function trg_members_count_del()
returns trigger
language plpgsql
as $$
begin
  if old.left_at is null then
update chats set members_count = greatest(members_count - 1, 0) where id = old.chat_id;
end if;
return null;
end;
$$;

drop trigger if exists members_count_ins on chat_members;
create trigger members_count_ins
    after insert on chat_members
    for each row
    execute function trg_members_count_ins();

drop trigger if exists members_count_upd on chat_members;
create trigger members_count_upd
    after update on chat_members
    for each row
    execute function trg_members_count_upd();

drop trigger if exists members_count_del on chat_members;
create trigger members_count_del
    after delete on chat_members
    for each row
    execute function trg_members_count_del();