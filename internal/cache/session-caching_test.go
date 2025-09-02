// Package cache provides session caching functionality for gateway session IDs and MCP session IDs.
package cache

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	// Test data
	sessions := map[string]map[string]struct {
		upstreamID string
		err        error
	}{
		"serverA": {
			"downstreamID1": {
				upstreamID: "upstreamID1A",
				err:        nil,
			},
		},
		"serverB": {
			"downstreamID1": {
				upstreamID: "upstreamID1B",
				err:        nil,
			},
		},
	}
	cache := New(func(_ context.Context, serverName, _, gwSessionID string) (mcpSessionID string, err error) {
		serverSessions, ok := sessions[serverName]
		if !ok {
			return "", fmt.Errorf("Unknown server %s", gwSessionID)
		}

		session, ok := serverSessions[gwSessionID]
		if !ok {
			return "", fmt.Errorf("Unknown test session %s", gwSessionID)
		}
		return session.upstreamID, session.err
	})

	ctx := context.Background()

	// Verify we can init
	upstreamID, err := cache.GetOrInit(ctx, "serverA", "", "downstreamID1")
	require.NoError(t, err)
	require.Equal(t, "upstreamID1A", upstreamID)

	// Verify we can have a session that already exists
	upstreamID, err = cache.GetOrInit(ctx, "serverA", "", "downstreamID1")
	require.NoError(t, err)
	require.Equal(t, "upstreamID1A", upstreamID)

	// Verify we can invalidate
	cache.Invalidate("serverA", "downstreamID1A")

	// Verify we can re-register
	upstreamID, err = cache.GetOrInit(ctx, "serverA", "", "downstreamIDUnk")
	require.Error(t, err)
	require.Equal(t, "", upstreamID)

	// Verify server B can register
	upstreamID, err = cache.GetOrInit(ctx, "serverB", "", "downstreamID1")
	require.NoError(t, err)
	require.Equal(t, "upstreamID1B", upstreamID)

	// Verify server A still connected
	upstreamID, err = cache.GetOrInit(ctx, "serverA", "", "downstreamID1")
	require.NoError(t, err)
	require.Equal(t, "upstreamID1A", upstreamID)
}
