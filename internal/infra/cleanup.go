package infra

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ──────────────────────────────────────────────────────
// Constants
// ──────────────────────────────────────────────────────

const (
	// Age threshold: remove entries older than 3 days.
	cleanupAgeDays = 3

	// Max retained entries per key type.
	maxInboxEntries   = 1000
	maxOutboxEntries  = 500
	maxTimelineEntries = 100

	// SCAN batch size.
	scanBatchSize = 100
)

// Key prefix patterns used by SCAN.
var cleanupPatterns = []string{
	"inbox:*",
	"outbox:*",
	"timeline:*",
	"conv_list:*",
}

// ──────────────────────────────────────────────────────
// StartCleanupTask — launches a periodic cleanup goroutine
// ──────────────────────────────────────────────────────

// StartCleanupTask starts a background goroutine that runs CleanupExpiredData
// on the given interval. It returns immediately.
// The interval parameter allows test overrides; in production pass 1 * time.Hour.
func StartCleanupTask(rdb *redis.Client, logger *zap.Logger, interval time.Duration) {
	go func() {
		// Run once immediately on startup, then periodically.
		CleanupExpiredData(rdb, logger)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			CleanupExpiredData(rdb, logger)
		}
	}()
}

// ──────────────────────────────────────────────────────
// CleanupExpiredData — single-pass scan + trim
// ──────────────────────────────────────────────────────

// CleanupExpiredData scans all inbox/outbox/timeline/conv_list keys and
// trims each by:
//   1. ZREMRANGEBYSCORE — remove entries older than 3 days
//   2. ZREMRANGEBYRANK  — cap max entries (inbox: 1000, outbox: 500, timeline: 100)
//      conv_list keys are only trimmed by time (no rank cap).
func CleanupExpiredData(rdb *redis.Client, logger *zap.Logger) {
	ctx := context.Background()
	threshold := time.Now().Add(-cleanupAgeDays * 24 * time.Hour).Unix()

	for _, pattern := range cleanupPatterns {
		prefix := strings.SplitN(pattern, ":*", 2)[0] // "inbox", "outbox", etc.

		var cursor uint64
		for {
			keys, nextCursor, err := rdb.Scan(ctx, cursor, pattern, scanBatchSize).Result()
			if err != nil {
				logger.Error("SCAN failed",
					zap.String("pattern", pattern),
					zap.Error(err),
				)
				break
			}

			for _, key := range keys {
				cleanKey(rdb, ctx, logger, key, prefix, threshold)
			}

			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}
}

// ──────────────────────────────────────────────────────
// cleanKey — apply time + rank trim to a single key
// ──────────────────────────────────────────────────────

func cleanKey(rdb *redis.Client, ctx context.Context, logger *zap.Logger, key string, prefix string, threshold int64) {
	// 1. Time-based trim: remove entries with score <= threshold
	maxScore := strconv.FormatInt(threshold, 10)
	if err := rdb.ZRemRangeByScore(ctx, key, "0", maxScore).Err(); err != nil {
		logger.Error("ZRemRangeByScore failed",
			zap.String("key", key),
			zap.Error(err),
		)
		return
	}

	// 2. Rank-based trim: cap max entries (conv_list has no rank cap)
	maxCount := maxEntriesForPrefix(prefix)
	if maxCount > 0 {
		card, err := rdb.ZCard(ctx, key).Result()
		if err != nil {
			logger.Error("ZCard failed",
				zap.String("key", key),
				zap.Error(err),
			)
			return
		}
		if card > int64(maxCount) {
			removeCount := card - int64(maxCount)
			if err := rdb.ZRemRangeByRank(ctx, key, 0, removeCount-1).Err(); err != nil {
				logger.Error("ZRemRangeByRank failed",
					zap.String("key", key),
					zap.Error(err),
				)
			}
		}
	}
}

// maxEntriesForPrefix returns the max retained entries for a key prefix.
// Returns 0 for conv_list (no rank-based trim).
func maxEntriesForPrefix(prefix string) int {
	switch prefix {
	case "inbox":
		return maxInboxEntries
	case "outbox":
		return maxOutboxEntries
	case "timeline":
		return maxTimelineEntries
	case "conv_list":
		return 0 // no rank cap
	default:
		return 0
	}
}

// ──────────────────────────────────────────────────────
// extractIDFromKey — parse "prefix:id" into an int64
// ──────────────────────────────────────────────────────

func extractIDFromKey(key string) (int64, error) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return 0, fmt.Errorf("invalid key format: %s", key)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid ID in key %s: %w", key, err)
	}
	return id, nil
}
