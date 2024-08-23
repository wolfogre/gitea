// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package globallock

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocker(t *testing.T) {
	t.Run("redis", func(t *testing.T) {
		url := "redis://127.0.0.1:6379/0"
		if os.Getenv("CI") == "" {
			// Make it possible to run tests against a local redis instance
			url = os.Getenv("TEST_REDIS_URL")
			if url == "" {
				t.Skip("TEST_REDIS_URL not set and not running in CI")
				return
			}
		}
		oldExpiry := redisLockExpiry
		redisLockExpiry = 5 * time.Second // make it shorter for testing
		defer func() {
			redisLockExpiry = oldExpiry
		}()

		locker := NewRedisLocker(url)
		testLocker(t, locker)
		testRedisLocker(t, locker.(*redisLocker))
	})
	t.Run("memory", func(t *testing.T) {
		locker := NewMemoryLocker()
		testLocker(t, locker)
		testMemoryLocker(t, locker.(*memoryLocker))
	})
}

func testLocker(t *testing.T, locker Locker) {
	t.Run("lock", func(t *testing.T) {
		parentCtx := context.Background()
		ctx, release, err := locker.Lock(parentCtx, "test")
		defer release()

		assert.NotEqual(t, parentCtx, ctx) // new context should be returned
		assert.NoError(t, err)

		func() {
			parentCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			ctx, release, err := locker.Lock(parentCtx, "test")
			defer release()

			assert.Error(t, err)
			assert.Equal(t, parentCtx, ctx) // should return the same context
		}()

		release()
		assert.Error(t, ctx.Err())
		release() // should be safe to call multiple times

		func() {
			_, release, err := locker.Lock(context.Background(), "test")
			defer release()

			assert.NoError(t, err)
		}()
	})

	t.Run("try lock", func(t *testing.T) {
		parentCtx := context.Background()
		ok, ctx, release, err := locker.TryLock(parentCtx, "test")
		defer release()

		assert.True(t, ok)
		assert.NotEqual(t, parentCtx, ctx) // new context should be returned
		assert.NoError(t, err)

		func() {
			parentCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			ok, ctx, release, err := locker.TryLock(parentCtx, "test")
			defer release()

			assert.False(t, ok)
			assert.NoError(t, err)
			assert.Equal(t, parentCtx, ctx) // should return the same context
		}()

		release()
		assert.Error(t, ctx.Err())
		release() // should be safe to call multiple times

		func() {
			ok, _, release, _ := locker.TryLock(context.Background(), "test")
			defer release()

			assert.True(t, ok)
		}()
	})

	t.Run("wait and acquired", func(t *testing.T) {
		ctx := context.Background()
		ctx, release, err := locker.Lock(ctx, "test")
		require.NoError(t, err)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			started := time.Now()
			_, release, err := locker.Lock(context.Background(), "test") // should be blocked for seconds
			defer release()
			assert.Greater(t, time.Since(started), time.Second)
			assert.NoError(t, err)
		}()

		time.Sleep(2 * time.Second)
		release()

		wg.Wait()
	})

	t.Run("continue after release", func(t *testing.T) {
		ctx := context.Background()

		ctxBeforeLock := ctx
		ctx, release, err := locker.Lock(ctx, "test")

		require.NoError(t, err)
		assert.NoError(t, ctx.Err())
		assert.NotEqual(t, ctxBeforeLock, ctx)

		ctxBeforeRelease := ctx
		ctx = release()

		assert.NoError(t, ctx.Err())
		assert.Error(t, ctxBeforeRelease.Err())

		// so it can continue with ctx to do more work
	})
}

// testMemoryLocker does specific tests for memoryLocker
func testMemoryLocker(t *testing.T, locker *memoryLocker) {
	// nothing to do
}

// testRedisLocker does specific tests for redisLocker
func testRedisLocker(t *testing.T, locker *redisLocker) {
	t.Run("missing extension", func(t *testing.T) {
		ctx, release, err := locker.Lock(context.Background(), "test")
		defer release()
		require.NoError(t, err)

		// It simulates that there are some problems with extending like network issues or redis server down.
		v, ok := locker.mutexM.Load("test")
		require.True(t, ok)
		m := v.(*redisMutex)
		_, _ = m.mutex.Unlock() // release it to make it impossible to extend

		select {
		case <-time.After(redisLockExpiry + time.Second):
			t.Errorf("lock should be expired")
		case <-ctx.Done():
			var errTaken *redsync.ErrTaken
			assert.ErrorAs(t, context.Cause(ctx), &errTaken)
		}
	})
}
