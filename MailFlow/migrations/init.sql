-- 创建数据库（如果需要）
-- CREATE DATABASE mailflow;

-- 连接到数据库
-- \c mailflow;

-- 注意：表结构会由GORM自动创建
-- 这个文件仅用于参考和手动创建数据库

-- API密钥表
-- CREATE TABLE IF NOT EXISTS api_keys (
--     id SERIAL PRIMARY KEY,
--     key VARCHAR(255) UNIQUE NOT NULL,
--     name VARCHAR(255) NOT NULL,
--     rate_limit INTEGER DEFAULT 100,
--     daily_limit INTEGER DEFAULT 10000,
--     status VARCHAR(50) DEFAULT 'active',
--     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
--     updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
-- );

-- SMTP配置表
-- CREATE TABLE IF NOT EXISTS smtp_configs (
--     id SERIAL PRIMARY KEY,
--     name VARCHAR(255) NOT NULL,
--     host VARCHAR(255) NOT NULL,
--     port INTEGER NOT NULL,
--     username VARCHAR(255) NOT NULL,
--     password VARCHAR(255) NOT NULL,
--     from_email VARCHAR(255) NOT NULL,
--     from_name VARCHAR(255),
--     max_per_hour INTEGER DEFAULT 100,
--     priority INTEGER DEFAULT 1,
--     status VARCHAR(50) DEFAULT 'active',
--     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
--     updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
-- );

-- 发送日志表
-- CREATE TABLE IF NOT EXISTS send_logs (
--     id SERIAL PRIMARY KEY,
--     api_key_id INTEGER,
--     to VARCHAR(255) NOT NULL,
--     subject VARCHAR(500),
--     status VARCHAR(50),
--     error_msg TEXT,
--     smtp_config_id INTEGER,
--     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
-- );

-- 用量统计表
-- CREATE TABLE IF NOT EXISTS usage_stats (
--     id SERIAL PRIMARY KEY,
--     api_key_id INTEGER NOT NULL,
--     date DATE NOT NULL,
--     sent_count INTEGER DEFAULT 0,
--     failed_count INTEGER DEFAULT 0,
--     updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
--     UNIQUE(api_key_id, date)
-- );

-- 创建索引
-- CREATE INDEX IF NOT EXISTS idx_api_keys_key ON api_keys(key);
-- CREATE INDEX IF NOT EXISTS idx_send_logs_api_key_id ON send_logs(api_key_id);
-- CREATE INDEX IF NOT EXISTS idx_send_logs_status ON send_logs(status);
-- CREATE INDEX IF NOT EXISTS idx_send_logs_created_at ON send_logs(created_at);
-- CREATE INDEX IF NOT EXISTS idx_usage_stats_apikey_date ON usage_stats(api_key_id, date);

