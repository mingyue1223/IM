CREATE TABLE IF NOT EXISTS friend_groups (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id     BIGINT NOT NULL,
    name        VARCHAR(30) NOT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_friend_group_name (user_id, name),
    INDEX idx_friend_group_user_sort (user_id, sort_order, id)
);

ALTER TABLE friendships
    ADD COLUMN group_id BIGINT NULL AFTER remark,
    ADD INDEX idx_friendship_group (user_id, group_id);

ALTER TABLE `groups`
    ADD COLUMN mute_all TINYINT(1) NOT NULL DEFAULT 0 AFTER max_members;
