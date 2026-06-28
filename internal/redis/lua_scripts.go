package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

// LoadLuaScripts preloads all Lua scripts into Redis for EVALSHA optimization.
// go-redis automatically caches SHA on first Eval call, so explicit preload
// is not strictly necessary. This function can be called at startup to warm
// the cache so that subsequent calls use EVALSHA instead of EVAL.
func LoadLuaScripts(rdb *goredis.Client, ctx context.Context) error {
	scripts := []string{
		luaPrivateMsgCheck,
		luaGroupMsgCheck,
		luaInboxMarkRead,
		luaRevokeMsg,
	}
	for _, s := range scripts {
		if err := rdb.ScriptLoad(ctx, s).Err(); err != nil {
			return err
		}
	}
	return nil
}
