/* Caches streams that users are permitted to see. */

package main

import (
	"container/list"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	
	uuid "github.com/pborman/uuid"
)

const MAX_CACHED uint64 = 4096 // Maximum number of streams that are cached

type TagInfo struct {
	name string
	permissions map[uuid.Array]bool // Maps UUID to permission bit. Protected by permcacheLock.
	element *list.Element
}

// Maps TAG to a struct describing its permissions
var permcache map[string]*TagInfo = make(map[string]*TagInfo)
var defaulttags = []string{ "public" }
var totalCached uint64 = 0

// This is a bit coarse: I don't really need to lock the whole permcache if I'm just changing one entry
// But hierarchical locking would be overkill here...
var permcacheLock sync.Mutex = sync.Mutex{}

// Use LRU policy: keep track of which tags are used most recently
var lruList *list.List = list.New()
var lruListLock sync.Mutex = sync.Mutex{}

func hasPermission(session *LoginSession, uuidBytes uuid.UUID) bool {
	var tags []string
	if session == nil {
		tags = defaulttags
	} else {
		tags = session.tags
	}
	uuidString := uuidBytes.String()
	for _, tag := range tags {
		if tagHasPermission(tag, uuidBytes, uuidString) {
			return true
		}
	}
	
	return false
}

func tagHasPermission(tag string, uuidBytes uuid.UUID, uuidString string) bool {
	var hasPerm bool
	
	uuidarr := uuidBytes.Array()
	permcacheLock.Lock()
	taginfo, ok := permcache[tag]
	if !ok {
		/* Cache Miss: never seen this token */
		taginfo = &TagInfo{ name: tag, permissions: make(map[uuid.Array]bool) }
		lruListLock.Lock()
		taginfo.element = lruList.PushFront(taginfo)
		lruListLock.Unlock()
		permcache[tag] = taginfo
	} else {
		lruListLock.Lock()
		lruList.MoveToFront(taginfo.element) // most recently used
		lruListLock.Unlock()
		hasPerm, ok = taginfo.permissions[uuidBytes.Array()]
		if ok {
			/* Cache Hit */
			permcacheLock.Unlock()
			return hasPerm
		}
		/* Cache Miss: never seen this UUID */
	}
	permcacheLock.Unlock()
	
	/* Ask the metadata server for the metadata of the corresponding stream. */
	query := fmt.Sprintf("select * where uuid = \"%s\";", uuidString)
	mdReq, err := http.NewRequest("POST", fmt.Sprintf("%s?tags=%s", mdServer, tag), strings.NewReader(query))
	if err != nil {
		return false
	}
	
	mdReq.Header.Set("Content-Type", "text")
	mdReq.Header.Set("Content-Length", fmt.Sprintf("%v", len(query)))
	resp, err := http.DefaultClient.Do(mdReq)
	
	if err != nil {
		return false
	}
	
	/* If the response is [] we lack permission; if it's longer we have permission. */
	buf := make([]byte, 3)
	n, err := io.ReadFull(resp.Body, buf)
	if n == 3 && buf[0] == '[' {
		hasPerm = true
	} else if n == 2 && err == io.ErrUnexpectedEOF && buf[0] == '[' && buf[1] == ']' {
		hasPerm = false
	} else {
		/* Server error. */
		fmt.Printf("Metadata server error: %v %c %c %c\n", n, buf[0], buf[1], buf[2])
		return false
	}
	
	/* If we didn't return early due to some kind of error, cache the result and return it. */
	permcacheLock.Lock()
	if taginfo.element != nil { // If this has been evicted from the cache, don't bother
		_, ok := taginfo.permissions[uuidarr]
		taginfo.permissions[uuidarr] = hasPerm // still update cached value
		if !ok { // If a different goroutine added it before we got here, then skip this part
			totalCached += 1
			if totalCached > MAX_CACHED {
				// Make this access return quickly, so start pruning in a new goroutine
				go pruneCache()
			}
		}
	}
	permcacheLock.Unlock()
	
	return hasPerm
}

func pruneCache() {
	permcacheLock.Lock()
	lruListLock.Lock()
	defer lruListLock.Unlock()
	defer permcacheLock.Unlock()
	
	if totalCached <= (MAX_CACHED >> 1) {
		// In case this gets invoked twice, make sure it only runs once
		return
	}
	
	var tag string
	var taginfo *TagInfo
	
	for tag, taginfo = range permcache {
		for key, perm := range taginfo.permissions {
			if !perm {
				delete(taginfo.permissions, key)
				totalCached -= 1
			}
		}
		if len(taginfo.permissions) == 0 {
			lruList.Remove(taginfo.element)
			delete(permcache, tag)
			taginfo.element = nil
		}
	}
	
	var element *list.Element
	// Now, remove cached permissions until we're within the limit
	for totalCached > (MAX_CACHED >> 1) {
		element = lruList.Back() // least recently used
		lruList.Remove(element)
		taginfo = element.Value.(*TagInfo)
		delete(permcache, taginfo.name)
		totalCached -= uint64(len(taginfo.permissions))
		taginfo.element = nil // mark as discarded
	}
}
