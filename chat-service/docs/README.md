# Chat Service (gRPC)

## Описание
Сервис чата (telegram-like). Отвечает за:
- управление чатами/комнатами и участниками (DIRECT/GROUP/CHANNEL)
- получение/сохранение сообщений (история)
- отдачу списка чатов с **последним сообщением**
- realtime доставку событий через **bi-di streaming** (`Connect`)

![alt tag](https://github.com/byoverr/simple_chat/blob/develop/chat-service/docs/img/scheme.png "Схемка")

### Как подключаться
**gRPC адрес:** `localhost:50052`

### Аутентификация
Все методы защищены access JWT (HS256):
- кладётся в metadata: `authorization: Bearer <access_jwt>`
- claims: `sub` (user_id), `roles`, `iss`, `aud`, `exp`

 chat-service не выдаёт токены — он только валидирует access JWT (через interceptor).



## Методы

**Service:** `chat.v1.ChatService`  
**Формат заголовка:** `authorization: Bearer <access_jwt>`

### Unary
- **ListChats** — `ListChats(ListChatsRequest) -> ListChatsResponse`  
  Список чатов пользователя (аналог диалогов) **с последним сообщением** и (опц.) `unread_count`.

- **GetChat** — `GetChat(GetChatRequest) -> Chat`  
  Получить чат по `chat_id`.

- **CreateChat** — `CreateChat(CreateChatRequest) -> Chat`  
  Создать чат: `DIRECT | GROUP | CHANNEL`.

- **JoinChat** — `JoinChat(JoinChatRequest) -> google.protobuf.Empty`  
  Вступить в чат (группа/канал).

- **LeaveChat** — `LeaveChat(LeaveChatRequest) -> google.protobuf.Empty`  
  Выйти из чата.

- **GetHistory** — `GetHistory(GetHistoryRequest) -> GetHistoryResponse`  
  История сообщений с пагинацией “вверх” (`before_seq` / `before_message_id`).

- **GetLastMessage** — `GetLastMessage(GetLastMessageRequest) -> GetLastMessageResponse`  
  Получить только последнее сообщение чата (если нужно отдельно от `ListChats`).

### Streaming
- **Connect** — `Connect(stream ClientEvent) <-> (stream ServerEvent)`  
  Realtime соединение:
  - клиент шлёт: `SendMessage`, `Typing`, `Read`, `Ack`, `Ping`
  - сервер пушит: `MessageCreated`, `TypingEvent`, `ReadEvent`, `ChatUpdated`, `Pong`



## Сообщения (кратко)

- `ListChatsRequest { limit, page_token }`
- `ListChatsResponse { items: ChatSummary[], next_page_token }`

- `Chat { id, type, title, avatar_url?, created_by, created_at, members_count }`
- `ChatSummary { chat, last_message: MessagePreview?, unread_count?, last_read_message_id? }`

- `CreateChatRequest { type, title?, member_user_ids[] }`
- `JoinChatRequest { chat_id }`
- `LeaveChatRequest { chat_id }`

- `GetHistoryRequest { chat_id, limit, before_message_id? | before_seq? }`
- `GetHistoryResponse { messages[], has_more, next_before_message_id, next_before_seq }`

- `GetLastMessageRequest { chat_id }`
- `GetLastMessageResponse { message? }`

### Streaming payloads
- `ClientEvent { SendMessage | Typing | Read | Ack | Ping }`
- `ServerEvent { MessageCreated | TypingEvent | ReadEvent | ChatUpdated | Pong | Error }`

Ключевые поля:
- `SendMessage { chat_id, client_msg_id, text, reply_to_message_id? }`
- `Message { id, chat_id, sender_id, client_msg_id, text, seq, created_at, edited_at?, reply_to_message_id? }`



## Хранилище (chatdb) — базовая схема

- `chats` — метаданные чатов + денормализация “последнего сообщения”
  - `last_message_id`, `last_message_seq` (для быстрого `ListChats`)

- `chat_members` — участники чатов (PK: `chat_id,user_id`)

- `messages` — сообщения с порядком и идемпотентностью
  - `unique(chat_id, client_msg_id)` — защита от дублей при ретраях
  - `index(chat_id, seq desc)` — быстрый `GetHistory`

- `chat_state` (опционально) — состояние прочтения для `unread_count`
  - `last_read_seq` на пользователя и чат



## Ошибки (маппинг в gRPC codes)

- `InvalidArgument` — некорректный/пустой ввод (chat_id, text, limit и т.п.)
- `Unauthenticated` — отсутствует/невалидный/истёкший access JWT
- `PermissionDenied` — нет доступа к чату (не участник / запрещено писать в канал)
- `NotFound` — чат/сообщение не найдено
- `AlreadyExists` — попытка создать дубликат (например, DIRECT чат уже существует)
- `ResourceExhausted` — backpressure/слишком медленный клиент в realtime (переполнен буфер)
- `Internal` — неожиданные ошибки