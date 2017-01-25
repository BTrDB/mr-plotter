/*
 * Copyright (C) 2016 Sam Kumar, Michael Andersen, and the University
 * of California, Berkeley.
 *
 * This file is part of Mr. Plotter (the Multi-Resolution Plotter).
 *
 * Mr. Plotter is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Mr. Plotter is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with Mr. Plotter.  If not, see <http://www.gnu.org/licenses/>.
 */

/* Caches permissions. */

package main

import (
	"container/list"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"gopkg.in/btrdb.v4"

	"github.com/SoftwareDefinedBuildings/mr-plotter/accounts"
	"github.com/pborman/uuid"
)

var max_cached uint64 // Maximum number of tag permissions that are cached

type TagPermissionQuery struct {
	tagname string
	uu uuid.Array
}

type TagPermission struct {
	query TagPermissionQuery
	hasPermission bool
	queryPending bool
	requestFailed bool
	queryPendingCond *sync.Cond
	element *list.Element
}

// Maps TAG to a struct describing its permissions
var permcache map[TagPermissionQuery]*TagPermission = make(map[TagPermissionQuery]*TagPermission)
var defaulttags = []string{ accounts.PUBLIC_TAG }
var totalCached uint64 = 0

// This is a bit coarse: I don't really need to lock the whole permcache if I'm just changing one entry
// But hierarchical locking would be overkill here...
var permcacheLock sync.Mutex = sync.Mutex{}
var pruningCache bool = false // protected by permcacheLock

// Use LRU policy: keep track of which tags are used most recently
var lruList *list.List = list.New()
var lruListLock sync.Mutex = sync.Mutex{}

// Lock ordering is to always acquire the permcacheLock before the lruListLock

func setTagPermissionCacheSize(maxCached uint64) {
	permcacheLock.Lock()
	lruListLock.Lock()
	max_cached = maxCached
	pruneCacheIfNecessary()
	lruListLock.Unlock()
	permcacheLock.Unlock()
}

// the permcacheLock and lruListLock must be held when this function executes
func pruneCacheIfNecessary() {
	var element *list.Element
	var taginfo *TagPermission
	for totalCached > max_cached {
		element = lruList.Back() // least recently used
		lruList.Remove(element)
		taginfo = element.Value.(*TagPermission)
		delete(permcache, taginfo.query)
		totalCached -= 1
	}
}

func hasPermission(ctx context.Context, session *LoginSession, uuidBytes uuid.UUID) bool {
	var tags []string

	/* First, check if the stream exists. */
	var s *btrdb.Stream = btrdbConn.StreamFromUUID(uuidBytes)
	if ok, err := s.Exists(ctx); !ok || err != nil {
		/* We don't want to cache this result. That way, we don't have to worry
		 * about invalidating the cache if a stream is created.
		 */
		return false
	}

	if session == nil {
		tags = defaulttags
	} else {
		if _, hasall := session.Tags[accounts.ALL_TAG]; hasall {
			return true
		}
		tags = session.TagSlice()
	}
	uuidString := uuidBytes.String()
	for _, tag := range tags {
		if tagHasPermission(ctx, tag, uuidBytes, uuidString, s) {
			return true
		}
	}

	return false
}

func checkforpermission(tag string, uuidString string) (bool, error) {
	var hasPerm bool

	/* Ask the metadata server for the metadata of the corresponding stream. */
	query := fmt.Sprintf("select * where uuid = \"%s\";", uuidString)
	mdReq, err := http.NewRequest("POST", fmt.Sprintf("%s?tags=%s", mdServer, tag), strings.NewReader(query))
	if err != nil {
		return false, err
	}

	mdReq.Header.Set("Content-Type", "text")
	mdReq.Header.Set("Content-Length", fmt.Sprintf("%v", len(query)))
	resp, err := http.DefaultClient.Do(mdReq)

	if err != nil {
		return false, err
	}

	/* If the response is [] we lack permission; if it's longer we have permission. */
	buf := make([]byte, 3)
	n, err := io.ReadFull(resp.Body, buf)
	resp.Body.Close()

	if n == 3 && buf[0] == '[' {
		hasPerm = true
	} else if n == 2 && err == io.ErrUnexpectedEOF && buf[0] == '[' && buf[1] == ']' {
		hasPerm = false
	} else {
		/* Server error. */
		return false, fmt.Errorf("Metadata server error: %v %c %c %c", n, buf[0], buf[1], buf[2])
	}

	return hasPerm, nil
}

func tagHasPermission(ctx context.Context, tag string, uuidBytes uuid.UUID, uuidString string, s *btrdb.Stream) bool {
	var hasPerm bool = false
	var err error

	var query TagPermissionQuery = TagPermissionQuery{tagname: tag, uu: uuidBytes.Array()}

	permcacheLock.Lock()
	taginfo, ok := permcache[query]
	if ok {
		/* Wait for the result if it's still pending. */
		for taginfo.queryPending {
			taginfo.queryPendingCond.Wait()
		}
		if taginfo.requestFailed {
			/* The request for the data failed, so just return false. */
			/* Alternatively, we could work out a way to try again for this request,
			 * but I think it's a bad idea. */
			return false
		}
		/* Cache hit. */
		lruListLock.Lock()
		lruList.MoveToFront(taginfo.element) // most recently used
		lruListLock.Unlock()
		hasPerm = taginfo.hasPermission
		permcacheLock.Unlock()
		return hasPerm
	}

	/* Cache Miss: never seen this query */
	taginfo = &TagPermission{ query: query, queryPending: true, requestFailed: false, queryPendingCond: sync.NewCond(&permcacheLock) }
	permcache[query] = taginfo
	permcacheLock.Unlock()

	/* Make a request to the underlying database. */
	if len(mdServer) == 0 {
		// Query BTrDB
		var collection string
		collection, err = s.Collection(ctx)
		if err == nil {
			var tagdef *accounts.MrPlotterTagDef
			tagdef, err = accounts.RetrieveTagDef(ctx, etcdConn, tag)
			if err == nil {
				for pfx := range tagdef.PathPrefix {
					hasPerm = strings.HasPrefix(collection, pfx)
					if hasPerm {
						break
					}
				}
			}
		}
	} else {
		// Query the metadata server
		hasPerm, err = checkforpermission(tag, uuidString)
	}
	if err != nil {
		log.Printf("Request for tag permission failed: %v", err)
		permcacheLock.Lock()
		delete(permcache, taginfo.query)
		taginfo.queryPending = false
		taginfo.requestFailed = true
		taginfo.queryPendingCond.Broadcast()
		permcacheLock.Unlock()
		return false
	}

	/* If we didn't return early due to some kind of error, cache the result and return it. */
	permcacheLock.Lock()
	taginfo.hasPermission = hasPerm
	taginfo.queryPending = false
	taginfo.queryPendingCond.Broadcast()

	/* Actually add this to the LRU list. Maybe this should happen before we make the query to get the permission? */
	lruListLock.Lock()
	taginfo.element = lruList.PushFront(taginfo)
	totalCached += 1
	pruneCacheIfNecessary()
	lruListLock.Unlock()

	permcacheLock.Unlock()

	return hasPerm
}
