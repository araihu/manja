package cache

import "sync"

type Memory struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemory() *Memory {
	return &Memory{data: map[string][]byte{}}
}

func (c *Memory) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.data[key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), value...), true
}

func (c *Memory) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = append([]byte(nil), value...)
}

func (c *Memory) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}
