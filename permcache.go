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
	"context"
	"log"
	"strings"

	"gopkg.in/btrdb.v4"

	"github.com/BTrDB/mr-plotter/accounts"
	etcd "github.com/coreos/etcd/clientv3"
	"github.com/pborman/uuid"
	"github.com/samkumar/reqcache"
)

type TagPermissionQuery struct {
	tagname string
	uu      uuid.Array
}

var defaulttags = []string{accounts.PublicTag}
var permcache = reqcache.NewLRUCache(1024, queryPermission, nil)
var prefixcache = reqcache.NewLRUCache(1024, queryPrefixes, nil)

func permCacheDaemon(ctx context.Context, ec *etcd.Client) {
	permCachePrefix := accounts.GetTagEtcdPath()
	watchchan := ec.Watch(ctx, permCachePrefix, etcd.WithPrefix())

	// At some point we may want to consider using a data structure for the
	// permission cache that is more efficient for this operation. For now,
	// I'm going to leave it as is because changing tag definitions should
	// be relatively rare.
	for watchresp := range watchchan {
		err := watchresp.Err()
		if err != nil {
			log.Fatalf("Error watching tags: %v", err)
		}
		permcache.Invalidate()
	}

	log.Fatalln("Watch on tags was lost")
}

func setTagPermissionCacheSize(newMaxCached uint64) {
	permcache.SetCapacity(newMaxCached)
}

func hasPermission(ctx context.Context, session *LoginSession, uuidBytes uuid.UUID) bool {
	var tags []string

	/* First, check if the stream exists. */
	s := btrdbConn.StreamFromUUID(uuidBytes)
	if ok, err := s.Exists(ctx); !ok || err != nil {
		/* We don't want to cache this result. That way, we don't have to worry
		 * about invalidating the cache if a stream is created.
		 */
		return false
	}

	if session == nil {
		tags = defaulttags
	} else {
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

func tagHasPermission(ctx context.Context, tag string, uuidBytes uuid.UUID, uuidString string, s *btrdb.Stream) bool {
	query := TagPermissionQuery{tagname: tag, uu: uuidBytes.Array()}
	hasPerm, err := permcache.Get(ctx, query)
	if err != nil {
		log.Printf("Could not request tag data: %v", err)
		return false
	}
	return hasPerm.(bool)
}

func queryPermission(ctx context.Context, key interface{}) (interface{}, uint64, error) {
	query := key.(TagPermissionQuery)
	s := btrdbConn.StreamFromUUID(query.uu.UUID())
	coll, err := s.Collection(ctx)
	if err != nil {
		return nil, 0, err
	}
	prefixes, err := prefixcache.Get(ctx, query.tagname)
	if err != nil {
		return nil, 0, err
	}
	for pfx := range prefixes.(map[string]struct{}) {
		if strings.HasPrefix(coll, pfx) {
			return true, 1, nil
		}
	}
	return false, 1, nil
}

func queryPrefixes(ctx context.Context, key interface{}) (interface{}, uint64, error) {
	tag := key.(string)
	tagdef, err := accounts.RetrieveTagDef(ctx, etcdConn, tag)
	if err != nil {
		return nil, 0, err
	}
	if tagdef == nil {
		return map[string]struct{}{}, 0, nil
	}
	return tagdef.PathPrefix, uint64(len(tagdef.PathPrefix)), nil
}
