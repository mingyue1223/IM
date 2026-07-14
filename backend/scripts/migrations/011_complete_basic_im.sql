ALTER TABLE friendships
    ADD COLUMN remark VARCHAR(50) NOT NULL DEFAULT '' AFTER friend_id;

ALTER TABLE private_messages
    ADD COLUMN reply_to_msg_id BIGINT NULL AFTER msg_type,
    ADD INDEX idx_private_reply (reply_to_msg_id);

ALTER TABLE group_messages
    ADD COLUMN reply_to_msg_id BIGINT NULL AFTER msg_type,
    ADD INDEX idx_group_reply (reply_to_msg_id);

CREATE TABLE IF NOT EXISTS attachments (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id     BIGINT NOT NULL,
    file_name   VARCHAR(255) NOT NULL,
    file_path   VARCHAR(500) NOT NULL,
    url         VARCHAR(500) NOT NULL,
    mime_type   VARCHAR(150) NOT NULL,
    size        BIGINT NOT NULL,
    kind        VARCHAR(20) NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_attachment_user_time (user_id, created_at)
);
