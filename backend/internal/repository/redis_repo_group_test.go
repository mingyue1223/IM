package repository

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveGroupMemberRedisClearsAuthorizationState(t *testing.T) {
	addr := os.Getenv("GOIM_TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:16379"
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: addr, DB: 1})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis unavailable: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := context.Background()
	groupID := int64(987654321)
	userID := int64(123456789)
	groupIDString := strconv.FormatInt(groupID, 10)
	userIDString := strconv.FormatInt(userID, 10)
	groupMembersKey := fmt.Sprintf("group_members:%d", groupID)
	userGroupsKey := fmt.Sprintf("user_groups:%d", userID)
	memberInfoKey := fmt.Sprintf("group_member_info:%d", groupID)
	memberRoleKey := fmt.Sprintf("group_member_role:%d", groupID)
	t.Cleanup(func() {
		rdb.Del(ctx, groupMembersKey, userGroupsKey, memberInfoKey, memberRoleKey)
	})

	require.NoError(t, rdb.SAdd(ctx, groupMembersKey, userIDString).Err())
	require.NoError(t, rdb.SAdd(ctx, userGroupsKey, groupIDString).Err())
	require.NoError(t, rdb.HSet(ctx, memberInfoKey, userIDString, `{"mutedUntil":9999999999999}`).Err())
	require.NoError(t, rdb.HSet(ctx, memberRoleKey, userIDString, 1).Err())

	repo := NewRedisRepo(rdb)
	require.NoError(t, repo.RemoveGroupMemberRedis(ctx, groupID, userID))

	assert.False(t, rdb.SIsMember(ctx, groupMembersKey, userIDString).Val())
	assert.False(t, rdb.SIsMember(ctx, userGroupsKey, groupIDString).Val())
	assert.False(t, rdb.HExists(ctx, memberInfoKey, userIDString).Val())
	assert.False(t, rdb.HExists(ctx, memberRoleKey, userIDString).Val())
}
