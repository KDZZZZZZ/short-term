-- Local development databases and per-service accounts.
--
-- Runs once, on an empty data directory, through the postgres image entrypoint.
-- Passwords here are local-only development values; deployed environments
-- inject credentials from the runtime environment instead.

CREATE ROLE account_svc     LOGIN PASSWORD 'account_svc';
CREATE ROLE marketplace_svc LOGIN PASSWORD 'marketplace_svc';
CREATE ROLE messaging_svc   LOGIN PASSWORD 'messaging_svc';
CREATE ROLE favorite_svc    LOGIN PASSWORD 'favorite_svc';

CREATE DATABASE account_db     OWNER account_svc;
CREATE DATABASE marketplace_db OWNER marketplace_svc;
CREATE DATABASE messaging_db   OWNER messaging_svc;
CREATE DATABASE favorite_db    OWNER favorite_svc;

-- PostgreSQL grants CONNECT to PUBLIC by default. Revoking it and granting it
-- back to the single owner is what makes cross-service access impossible: any
-- other service account is refused at connection time.
REVOKE CONNECT ON DATABASE account_db     FROM PUBLIC;
REVOKE CONNECT ON DATABASE marketplace_db FROM PUBLIC;
REVOKE CONNECT ON DATABASE messaging_db   FROM PUBLIC;
REVOKE CONNECT ON DATABASE favorite_db    FROM PUBLIC;

GRANT CONNECT ON DATABASE account_db     TO account_svc;
GRANT CONNECT ON DATABASE marketplace_db TO marketplace_svc;
GRANT CONNECT ON DATABASE messaging_db   TO messaging_svc;
GRANT CONNECT ON DATABASE favorite_db    TO favorite_svc;
