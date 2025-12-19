package mcache

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session represents a single client connection session.
type Session struct {
	ID        string
	SseChan   chan []byte // Channel to send SSE messages to the client
	CreatedAt time.Time
}

// Cache is a thread-safe in-memory store for client sessions.
type Cache struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewCache creates a new session cache.
func NewCache() *Cache {
	return &Cache{
		sessions: make(map[string]*Session),
	}
}

// NewSession creates a new session, stores it, and returns it.
func (c *Cache) NewSession() *Session {
	c.mu.Lock()
	defer c.mu.Unlock()

	session := &Session{
		ID:        uuid.NewString(),
		SseChan:   make(chan []byte, 10), // Buffered channel
		CreatedAt: time.Now(),
	}
	c.sessions[session.ID] = session
	return session
}

// GetSession retrieves a session by its ID.
func (c *Cache) GetSession(id string) (*Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	session, ok := c.sessions[id]
	return session, ok
}

// DeleteSession removes a session from the cache.
func (c *Cache) DeleteSession(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if session, ok := c.sessions[id]; ok {
		close(session.SseChan) // Close the channel to signal the SSE handler to stop
		delete(c.sessions, id)
	}
}
