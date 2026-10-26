# Auth Service (gRPC)

## Описание
Сервис аутентификации и авторизации пользователей. Он отвечает за регистрацию, вход в систему, управление сессиями и выдачу токенов доступа.

![alt tag](https://github.com/byoverr/simple-chat/blob/develop/auth-service/docs/img/scheme.png)


### Как подключаться

**gRPC адрес:** `localhost:50051`

### Аутентификация

- **Access token:** JWT (HS256)
    - кладётся в metadata: `authorization: Bearer <access_jwt>`
    - основные claims: `sub` (user_id), `roles`, `iss`, `aud`, `exp`
- **Refresh token:** opaque строка
    - **сырой refresh в БД не хранится**
    - в БД хранится `token_hash = HMAC-SHA256(refresh, REFRESH_PEPPER)`
    - **rotation:** при `Refresh()` старый refresh становится `revoked`, выдаётся новый


### Методы

**Service:** `auth.v1.AuthService`  
**Формат заголовка для защищённых методов:** `authorization: Bearer <access_jwt>`

#### Public (access token не нужен)
- **Register** — `Register(RegisterRequest) -> AuthPair`  
  Регистрация пользователя и выдача пары токенов `{access, refresh}`.
- **Login** — `Login(LoginRequest) -> AuthPair`  
  Логин по email/паролю и выдача пары токенов.
- **Refresh** — `Refresh(RefreshRequest) -> AuthPair`  
  Обновление токенов по refresh (rotation: старый refresh помечается revoked, выдаётся новая пара).
- **Logout** — `Logout(LogoutRequest) -> google.protobuf.Empty`  
  Выход по refresh-токену (отзыв refresh-сессии). Идемпотентно.

#### Protected (нужен access token)
- **WhoAmI** — `WhoAmI(google.protobuf.Empty) -> WhoAmIResponse`  
  Возвращает профиль текущего пользователя (user_id берётся из `sub` access JWT).
- **LogoutAll** — `LogoutAll(google.protobuf.Empty) -> google.protobuf.Empty`  
  Отзывает все refresh-сессии пользователя (logout на всех устройствах).

### Сообщения (кратко)

- `RegisterRequest { email, password, display_name, client? }`
- `LoginRequest { email, password, client? }`
- `RefreshRequest { refresh_token, client? }`
- `LogoutRequest { refresh_token }`

- `AuthPair {`
    - `access_token`
    - `access_expires_at`
    - `refresh_token`
    - `refresh_expires_at`
    - `user_id`
    - `session_id`
    - `roles[]`
      `}`

- `WhoAmIResponse { user_id, email, display_name, roles[] }`

### Ошибки (маппинг в gRPC codes)

- `InvalidArgument` — некорректный/пустой ввод (например, плохой email)
- `AlreadyExists` — email уже зарегистрирован
- `Unauthenticated` — неверные креды / невалидный или истёкший access / revoked refresh
- `PermissionDenied` — пользователь отключён (disabled)
- `ResourceExhausted` — сработал rate limit (login/register/refresh)
- `Internal` — неожиданные ошибки

