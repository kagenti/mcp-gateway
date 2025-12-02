package session

import (
	"context"
	"sync"

	redis "github.com/redis/go-redis/v9"
)

// Cache implements a cache
type Cache struct {
	connectionString string
	inmemory         *sync.Map
	extClient        *redis.Client
}

// KeyExists checks if a key exists in the cache
func (c *Cache) KeyExists(ctx context.Context, key string) (bool, error) {
	if c.inmemory != nil {
		_, ok := c.inmemory.Load(key)
		return ok, nil
	}
	count, err := c.extClient.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	return false, nil

}

// GetSession returns a session from the cache
func (c *Cache) GetSession(ctx context.Context, key string) (map[string]string, error) {
	if c.inmemory != nil {
		val, ok := c.inmemory.Load(key)
		if ok {
			return val.(map[string]string), nil
		}
		return map[string]string{}, nil
	}
	return c.extClient.HGetAll(ctx, key).Result()
}

// DeleteSessions deletes sessions from the cache
func (c *Cache) DeleteSessions(ctx context.Context, key ...string) error {
	if c.inmemory != nil {
		for _, k := range key {
			c.inmemory.Delete(k)
		}
		return nil
	}
	return c.extClient.Del(ctx, key...).Err()
}

// AddSession will add a session under the key. If the key exists it will append that session
func (c *Cache) AddSession(ctx context.Context, key, mcpServerID, mcpSession string) (bool, error) {
	if c.inmemory != nil {
		session, err := c.GetSession(ctx, key)
		if err != nil {
			return false, err
		}
		session[mcpServerID] = mcpSession
		c.inmemory.Store(key, session)
		return true, nil
	}
	err := c.extClient.HSet(ctx, key, mcpServerID, mcpSession).Err()
	if err != nil {
		return false, err
	}
	return true, nil
}

// RemoveServerSession remove specific server session form cache
func (c *Cache) RemoveServerSession(ctx context.Context, key, mcpServerID string) error {
	if c.inmemory != nil {
		session, err := c.GetSession(ctx, key)
		if err != nil {
			return err
		}
		delete(session, mcpServerID)
		c.inmemory.Store(key, session)
		return nil
	}
	return c.extClient.HDel(ctx, key, mcpServerID).Err()
}

// Close closes the cache connection
func (c *Cache) Close() error {
	if c.inmemory != nil {
		return nil
	}
	return c.extClient.Close()
}

// NewCache returns a new cache
func NewCache(ctx context.Context, opts ...func(*Cache)) (*Cache, error) {
	c := &Cache{
		inmemory: nil,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.connectionString != "" {
		opt, err := redis.ParseURL(c.connectionString)
		if err != nil {
			return c, err
		}

		c.extClient = redis.NewClient(opt)
		return c, c.extClient.Ping(ctx).Err()
	}
	c.inmemory = &sync.Map{}
	return c, nil
}

// WithConnectionString accepts a redis connections string "redis://<user>:<pass>@localhost:6379/<db>"
func WithConnectionString(url string) func(c *Cache) {
	return func(c *Cache) {
		c.inmemory = nil
		c.connectionString = url
	}
}
