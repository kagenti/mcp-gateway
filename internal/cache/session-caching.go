// Package cache provides session caching functionality for gateway session IDs and MCP session IDs.
package cache

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// sessionInitialiser is a function that initializes a new MCP session for a given server and gateway session.
type sessionInitialiser func(ctx context.Context, serverName string, authority string, gwSessionID string) (mcpSessionID string, err error)

// Cache manages MCP session ID mappings between gateway sessions ID and MCP server sessions IDs.
type Cache struct {
	sessions    sync.Map
	initSession sessionInitialiser
}

type key struct {
	serverName string
	gw         string
}

// New creates a new Cache instance with the provided session initializer function.
func New(initSession sessionInitialiser) *Cache {
	return &Cache{
		initSession: initSession,
	}
}

// GetOrInit retrieves an existing session or initializes a new one for the given server and gateway session.
func (c *Cache) GetOrInit(ctx context.Context, serverName string, authority string, gwSessionID string) (mcpSessionID string, err error) {
	k := key{serverName: serverName, gw: gwSessionID}

	// Check if session ID already exists
	if sessionID, exists := c.sessions.Load(k); exists {
		return sessionID.(string), nil
	}

	// Create new session if no session ID exists
	sessionID, err := c.initSession(ctx, serverName, authority, gwSessionID)
	if err != nil {
		return "", fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	// Store session ID, or get existing if another goroutine made the initialisation in the meantime
	if actual, loaded := c.sessions.LoadOrStore(k, sessionID); loaded {
		return actual.(string), nil
	}

	return sessionID, nil
}

// InvalidateByMCPSessionID removes the session entry that contains the given MCP session ID
func (c *Cache) InvalidateByMCPSessionID(mcpSessionID string) {
	// Log cache before deletion
	var beforeCount int
	c.sessions.Range(func(_, _ interface{}) bool {
		beforeCount++
		return true
	})
	slog.Debug("Before deletion", "count", beforeCount, "mcpSessionID", mcpSessionID)

	var keyToDelete *key
	c.sessions.Range(func(k, v interface{}) bool {
		if v.(string) == mcpSessionID {
			key := k.(key)
			keyToDelete = &key
			return false // stop iteration
		}
		return true // continue iteration
	})

	if keyToDelete != nil {
		c.sessions.Delete(*keyToDelete)

		// Log cache after deletion
		var afterCount int
		c.sessions.Range(func(_, _ interface{}) bool {
			afterCount++
			return true
		})
		slog.Debug("After deletion", "count", afterCount,
			"mcpSessionID", mcpSessionID,
			"serverName", keyToDelete.serverName,
			"gwSessionID", keyToDelete.gw)
	} else {
		slog.Debug("Session not found in cache", "mcpSessionID", mcpSessionID)
	}
}
