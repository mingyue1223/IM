CREATE TABLE IF NOT EXISTS ai_summaries (
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id       BIGINT NOT NULL,
    topic         VARCHAR(100) NOT NULL,
    key_points    JSON NOT NULL,
    conclusion    VARCHAR(500) NOT NULL,
    user_intent   VARCHAR(200) DEFAULT '',
    message_range JSON NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user_time (user_id, created_at)
);

CREATE TABLE IF NOT EXISTS ai_user_profiles (
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id       BIGINT NOT NULL,
    field_name    VARCHAR(50) NOT NULL,
    value         VARCHAR(200) NOT NULL,
    confidence    FLOAT NOT NULL,
    source        VARCHAR(50) NOT NULL,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_user_field (user_id, field_name),
    INDEX idx_user (user_id)
);
