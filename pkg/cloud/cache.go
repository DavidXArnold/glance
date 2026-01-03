package cloud

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
)

// cacheEntry holds cached cloud provider information with TTL.
type cacheEntry struct {
	InstanceType   string
	NodeGroup      string
	NodePool       string
	FargateProfile string
	CapacityType   string
	Timestamp      time.Time
}

// Cache holds the in-memory cloud info cache with TTL and optional disk
// persistence. The cache key is typically the full providerID string.
type Cache struct {
	mu      sync.RWMutex
	cache   map[string]*cacheEntry
	ttl     time.Duration
	useDisk bool
}

// NewCache creates a new cloud cache with the specified TTL and disk setting.
func NewCache(ttl time.Duration, useDisk bool) *Cache {
	c := &Cache{
		cache:   make(map[string]*cacheEntry),
		ttl:     ttl,
		useDisk: useDisk,
	}
	if useDisk {
		c.loadFromDisk()
	}
	return c
}

// Get retrieves a cached cloud info entry if it exists and is not expired.
func (c *Cache) Get(key string) (*Metadata, bool) {
	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Since(entry.Timestamp) > c.ttl {
		return nil, false
	}

	return &Metadata{
		InstanceType:   entry.InstanceType,
		NodeGroup:      entry.NodeGroup,
		NodePool:       entry.NodePool,
		FargateProfile: entry.FargateProfile,
		CapacityType:   entry.CapacityType,
	}, true
}

// Set stores a cloud info entry in the cache.
func (c *Cache) Set(key string, metadata *Metadata) {
	c.mu.Lock()
	c.cache[key] = &cacheEntry{
		InstanceType:   metadata.InstanceType,
		NodeGroup:      metadata.NodeGroup,
		NodePool:       metadata.NodePool,
		FargateProfile: metadata.FargateProfile,
		CapacityType:   metadata.CapacityType,
		Timestamp:      time.Now(),
	}
	c.mu.Unlock()

	if c.useDisk {
		// Save to disk in background.
		go c.saveToDisk()
	}
}

// GetOrFetch returns cached metadata when available or uses the registered
// Provider for the given providerName to fetch it and populate the cache.
func (c *Cache) GetOrFetch(ctx context.Context, providerName, key string) (*Metadata, error) {
	if md, ok := c.Get(key); ok {
		return md, nil
	}

	provider := LookupProvider(providerName)
	if provider == nil {
		return nil, nil
	}

	md, err := provider.NodeMetadata(ctx, key)
	if err != nil || md == nil {
		return nil, err
	}

	c.Set(key, md)
	return md, nil
}

// getCachePath returns the path to the disk cache file.
func (c *Cache) getCachePath() string {
	home, err := homedir.Dir()
	if err != nil {
		log.Debugf("failed to get home directory for cloud cache: %v", err)
		return ""
	}
	return filepath.Join(home, ".glance", "cloud-cache.json")
}

// loadFromDisk loads cached cloud info from disk.
func (c *Cache) loadFromDisk() {
	cachePath := c.getCachePath()
	if cachePath == "" {
		return
	}

	// #nosec G304 - cachePath is computed from home directory, not user input
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debugf("failed to read cloud cache from disk: %v", err)
		}
		return
	}

	var diskCache map[string]*cacheEntry
	if err := json.Unmarshal(data, &diskCache); err != nil {
		log.Debugf("failed to unmarshal cloud cache: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range diskCache {
		if time.Since(entry.Timestamp) <= c.ttl {
			c.cache[key] = entry
		}
	}

	log.Debugf("loaded %d cloud cache entries from disk", len(c.cache))
}

// saveToDisk saves the current cache to disk.
func (c *Cache) saveToDisk() {
	cachePath := c.getCachePath()
	if cachePath == "" {
		return
	}

	cacheDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		log.Debugf("failed to create cache directory: %v", err)
		return
	}

	c.mu.RLock()
	data, err := json.Marshal(c.cache)
	c.mu.RUnlock()
	if err != nil {
		log.Debugf("failed to marshal cloud cache: %v", err)
		return
	}

	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		log.Debugf("failed to write cloud cache to disk: %v", err)
	}
}
