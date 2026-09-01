-- 本地开发数据库和各服务账户。
--
-- 由 postgres 镜像的入口点在空数据目录上运行一次。
-- 此处的密码仅用于本地开发；部署环境会从运行时环境注入凭据。

CREATE ROLE account_svc     LOGIN PASSWORD 'account_svc';
CREATE ROLE marketplace_svc LOGIN PASSWORD 'marketplace_svc';
CREATE ROLE messaging_svc   LOGIN PASSWORD 'messaging_svc';
CREATE ROLE favorite_svc    LOGIN PASSWORD 'favorite_svc';

CREATE DATABASE account_db     OWNER account_svc;
CREATE DATABASE marketplace_db OWNER marketplace_svc;
CREATE DATABASE messaging_db   OWNER messaging_svc;
CREATE DATABASE favorite_db    OWNER favorite_svc;

-- PostgreSQL 默认向 PUBLIC 授予 CONNECT。撤销该权限并仅授予数据库所有者，
-- 才能真正禁止跨服务访问：其他服务账户会在连接时被拒绝。
REVOKE CONNECT ON DATABASE account_db     FROM PUBLIC;
REVOKE CONNECT ON DATABASE marketplace_db FROM PUBLIC;
REVOKE CONNECT ON DATABASE messaging_db   FROM PUBLIC;
REVOKE CONNECT ON DATABASE favorite_db    FROM PUBLIC;

GRANT CONNECT ON DATABASE account_db     TO account_svc;
GRANT CONNECT ON DATABASE marketplace_db TO marketplace_svc;
GRANT CONNECT ON DATABASE messaging_db   TO messaging_svc;
GRANT CONNECT ON DATABASE favorite_db    TO favorite_svc;
