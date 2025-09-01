// Package cache provides session caching functionality for gateway session IDs and MCP session IDs.
package cache

import (
	"context"
	"fmt"
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

// Invalidate removes the session from the cache for the given server and gateway session.
func (c *Cache) Invalidate(serverName string, gwSessionID string) {
	k := key{serverName: serverName, gw: gwSessionID}
	c.sessions.Delete(k)
}
