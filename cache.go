// Simple caching library with expiration capabilities
package cache2go

import (
	"errors"
	"sync"
	"time"
)

// Structure that must be embedded in the object that should be cached with expiration
// If no expiration is desired this can be ignored
type XEntry struct {
	sync.Mutex
	key            string
	keepAlive      bool
	expireDuration time.Duration
	expiringSince  time.Time
	data           interface{}

	// Callback method triggered right before removing the item from the cache
	aboutToExpire  func(string)
}

type XCache struct {
	sync.RWMutex
	Name           string
	Items          map[string]*XEntry
	expTimer       *time.Timer
	expDuration    time.Duration
}

var (
	xcache = make(map[string]*XCache)
	xmux   sync.RWMutex
	cache  = make(map[string]interface{})
	mux    sync.RWMutex
)

// Expiration check loop, triggered by a self-adjusting timer
func expirationCheck(table string) {
	xmux.RLock()
	t := xcache[table]
	xmux.RUnlock()

	t.Lock()
	if t.expTimer != nil {
		t.expTimer.Stop()
	}

	// Take a copy of xcache so we can iterate over it without blocking the mutex
	cc := t.Items
	t.Unlock()

	// To be more accurate with timers, we would need to update 'now' on every
	// loop iteration. Not sure it's really efficient though.
	now := time.Now()
	smallestDuration := 0 * time.Second
	for key, c := range cc {
		if now.Sub(c.expiringSince) >= c.expireDuration {
			t.Lock()
			if c.aboutToExpire != nil {
				c.aboutToExpire(key)
			}
			delete(t.Items, key)
			t.Unlock()
		} else {
			if smallestDuration == 0 || c.ExpireDuration() < smallestDuration {
				smallestDuration = c.ExpireDuration() - now.Sub(c.ExpiringSince())
			}
		}
	}

	t.Lock()
	t.expDuration = smallestDuration
	if smallestDuration > 0 {
		t.expTimer = time.AfterFunc(smallestDuration, func() {
			go expirationCheck(table)
		})
	}
	t.Unlock()
}

// Mark entry to be kept for another expirationDuration period
func (xe *XEntry) KeepAlive() {
	xe.Lock()
	defer xe.Unlock()
	xe.expiringSince = time.Now()
}

// Returns this entry's expiration duration
func (xe *XEntry) ExpireDuration() time.Duration {
	xe.Lock()
	defer xe.Unlock()
	return xe.expireDuration
}

// Returns since when this entry is expiring
func (xe *XEntry) ExpiringSince() time.Time {
	xe.Lock()
	defer xe.Unlock()
	return xe.expiringSince
}

// Returns the value of this cached item
func (xe *XEntry) Data() interface{} {
	xe.Lock()
	defer xe.Unlock()
	return xe.data
}

// Returns how many items are currently stored in the expiration cache
func (xc *XCache) XCacheCount() int {
	xc.RLock()
	defer xc.RUnlock()

	return len(xc.Items)
}

func CacheTable(table string) *XCache {
	xmux.RLock()
	t, ok := xcache[table]
	xmux.RUnlock()

	if !ok {
		t = &XCache{
			Name: table,
			Items: make(map[string]*XEntry),
		}
		xmux.Lock()
		xcache[table] = t
		xmux.Unlock()
	}

	return t
}

// Adds an expiring key/value pair to the cache
// The last parameter abouToExpireFunc can be nil. Otherwise abouToExpireFunc
// will be called (with this item's key as its only parameter), right before
// removing this item from the cache.
func (xc *XCache) XCache(key string, expire time.Duration, data interface{}, aboutToExpireFunc func(string)) {
	entry := XEntry{}
	entry.keepAlive = true
	entry.key = key
	entry.expireDuration = expire
	entry.expiringSince = time.Now()
	entry.aboutToExpire = aboutToExpireFunc
	entry.data = data

	xc.Lock()
	xc.Items[key] = &entry
	expDur := xc.expDuration
	xc.Unlock()

	// If we haven't set up any expiration check timer or found a more imminent item
	if expDur == 0 || expire < expDur {
		expirationCheck(xc.Name)
	}
}

// Get an entry from the expiration cache and mark it to be kept alive
func (xc *XCache) GetXCached(key string) (*XEntry, error) {
	xc.RLock()
	defer xc.RUnlock()
	if r, ok := xc.Items[key]; ok {
		r.KeepAlive()
		return r, nil
	}
	return nil, errors.New("Key not found in cache")
}

// Delete all keys from expiraton cache
func (xc *XCache) XFlush() {
	xc.Lock()
	defer xc.Unlock()

	xc.Items = make(map[string]*XEntry)
	xc.expDuration = 0
	if xc.expTimer != nil {
		xc.expTimer.Stop()
	}

	xmux.Lock()
	defer xmux.Unlock()
	delete(xcache, xc.Name)
}

// Adds a non-expiring key/value pair to the cache
func Cache(key string, value interface{}) {
	mux.Lock()
	defer mux.Unlock()
	cache[key] = value
}

// Extracts a value for a given non-expiring key
func GetCached(key string) (v interface{}, err error) {
	mux.RLock()
	defer mux.RUnlock()
	if r, ok := cache[key]; ok {
		return r, nil
	}
	return nil, errors.New("Key not found in cache")
}

// Delete all keys from non-expiring cache
func Flush() {
	mux.Lock()
	defer mux.Unlock()

	cache = make(map[string]interface{})
}

// Returns how many items are currently stored in the non-expiring cache
func CacheCount() int {
	mux.RLock()
	defer mux.RUnlock()

	return len(cache)
}
