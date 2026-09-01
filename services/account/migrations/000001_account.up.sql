-- Account Service schema: student credentials and public profile.
CREATE TABLE accounts (
    id            text PRIMARY KEY,
    student_no    text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    nickname      text NOT NULL,
    wechat        text,
    qq            text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT accounts_student_no_format CHECK (student_no ~ '^[A-Za-z0-9_-]{4,32}$'),
    CONSTRAINT accounts_password_hash_not_empty CHECK (char_length(password_hash) > 0),
    CONSTRAINT accounts_nickname_len CHECK (char_length(nickname) BETWEEN 1 AND 50),
    CONSTRAINT accounts_wechat_len CHECK (wechat IS NULL OR char_length(wechat) BETWEEN 1 AND 64),
    CONSTRAINT accounts_qq_format CHECK (qq IS NULL OR qq ~ '^[0-9]{5,20}$')
);
