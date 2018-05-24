package main

import (
  "log"
  "strings"
	"context"
	"github.com/samkumar/reqcache"
	"github.com/pborman/uuid"
)

type CollectionQuery struct {
	uu      uuid.Array
}

var permcache = reqcache.NewLRUCache(1024, queryCollection, nil)

func hasPermission(ctx context.Context, session *LoginSession, uuidBytes uuid.UUID) bool {
  checkPublicGroup()
  coll, err := permcache.Get(ctx, CollectionQuery{uu: uuidBytes.Array()})
  if err != nil {
    log.Fatalf("error getting from permcache: %v", err)
  }
  if session == nil {
		for _, p := range publicGroup.Prefixes {
			if strings.HasPrefix(coll.(string), p) {
        return true
      }
		}
		return false
	}
  for pfx, _ := range session.Prefixes {
    if strings.HasPrefix(coll.(string), pfx) {
      return true
    }
  }
  return false
}

func queryCollection(ctx context.Context, key interface{}) (interface{}, uint64, error) {
	query := key.(CollectionQuery)
	s := btrdbConn.StreamFromUUID(query.uu.UUID())
	coll, err := s.Collection(ctx)
	if err != nil {
		return nil, 0, err
	}
	return coll, 1, nil
}
