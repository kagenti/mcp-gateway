package cache

import (
	"context"
	"fmt"
	"sync"
)

type sessionInitialiser func(ctx context.Context, serverName string, gwSessionID string) (mvpSessionID string, err error)

type Cache struct{
	sessions map[key]string
	initSession sessionInitialiser
	mu sync.RWMutex
}

type key struct{
	serverName string
	gw string
}

func New(initSession sessionInitialiser) *Cache {
	return &Cache{
		sessions: make(map[key]string),
		initSession: initSession,
	}
}


func(c *Cache) GetOrInit(ctx context.Context, serverName string, gwSessionID string)(mvpSessionID string, err error){
	k := key{serverName: serverName, gw: gwSessionID}
	
	// Check if session already exists
	c.mu.RLock()
	if sessionID, exists := c.sessions[k]; exists {
		c.mu.RUnlock()
		return sessionID, nil
	}
	c.mu.RUnlock()

	// Need to create new session - acquire write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check if it was created while waiting
	if sessionID, exists := c.sessions[k]; exists {
		return sessionID, nil
	}

	// Create new session
	sessionID, err := c.initSession(ctx, serverName, gwSessionID)
	if err != nil {
		return "", fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	// Store session id 
	c.sessions[k] = sessionID
	return sessionID, nil
}

	func (c *Cache) Invalidate(serverName string, gwSessionID string) {
		k := key{serverName: serverName, gw: gwSessionID}
		c.mu.Lock()
		delete(c.sessions, k)
		c.mu.Unlock()
	}
