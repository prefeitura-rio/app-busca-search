package services

import (
	"container/list"
	"sync"
	"time"
)

// Cache é a interface para serviços de cache
type Cache interface {
	Get(key string) interface{}
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string)
	Clear()
	Size() int
}

// cacheEntry representa uma entrada no cache
type cacheEntry struct {
	key        string
	value      interface{}
	expiration time.Time
}

// LRUCache implementa um cache LRU (Least Recently Used) thread-safe
type LRUCache struct {
	capacity int
	mu       sync.RWMutex
	cache    map[string]*list.Element
	lruList  *list.List
}

// NewLRUCache cria um novo cache LRU com a capacidade especificada
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		lruList:  list.New(),
	}
}

// Get recupera um valor do cache
func (c *LRUCache) Get(key string) interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, found := c.cache[key]; found {
		entry := element.Value.(*cacheEntry)

		// Verificar se expirou
		if time.Now().After(entry.expiration) {
			c.removeElement(element)
			return nil
		}

		// Mover para o final da lista (mais recentemente usado)
		c.lruList.MoveToBack(element)
		return entry.value
	}

	return nil
}

// Set adiciona ou atualiza um valor no cache
func (c *LRUCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiration := time.Now().Add(ttl)

	// Se a chave já existe, atualizar
	if element, found := c.cache[key]; found {
		c.lruList.MoveToBack(element)
		entry := element.Value.(*cacheEntry)
		entry.value = value
		entry.expiration = expiration
		return
	}

	// Se o cache está cheio, remover o item menos recentemente usado
	if c.lruList.Len() >= c.capacity {
		oldest := c.lruList.Front()
		if oldest != nil {
			c.removeElement(oldest)
		}
	}

	// Adicionar novo item
	entry := &cacheEntry{
		key:        key,
		value:      value,
		expiration: expiration,
	}
	element := c.lruList.PushBack(entry)
	c.cache[key] = element
}

// Delete remove um item do cache
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, found := c.cache[key]; found {
		c.removeElement(element)
	}
}

// Clear limpa todo o cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lruList.Init()
}

// Size retorna o número de itens no cache
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lruList.Len()
}

// removeElement remove um elemento da lista e do mapa (deve ser chamado com lock)
func (c *LRUCache) removeElement(element *list.Element) {
	c.lruList.Remove(element)
	entry := element.Value.(*cacheEntry)
	delete(c.cache, entry.key)
}

// CleanupExpired remove todos os itens expirados do cache
func (c *LRUCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	// Iterar pela lista e remover items expirados
	var next *list.Element
	for element := c.lruList.Front(); element != nil; element = next {
		next = element.Next()
		entry := element.Value.(*cacheEntry)

		if now.After(entry.expiration) {
			c.removeElement(element)
			removed++
		}
	}

	return removed
}

// StartCleanupRoutine inicia uma rotina de limpeza periódica
func (c *LRUCache) StartCleanupRoutine(interval time.Duration) *time.Ticker {
	ticker := time.NewTicker(interval)

	go func() {
		for range ticker.C {
			removed := c.CleanupExpired()
			if removed > 0 {
				// Log apenas se removeu algo
				// log.Printf("Cache cleanup: removed %d expired entries", removed)
			}
		}
	}()

	return ticker
}
