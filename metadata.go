/*
 * Copyright (C) 2017 Sam Kumar, Michael Andersen, and the University
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

/* Handles metadata requests. */

package main

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"gopkg.in/btrdb.v4"

	"github.com/SoftwareDefinedBuildings/mr-plotter/accounts"
	etcd "github.com/coreos/etcd/clientv3"
	"github.com/pborman/uuid"
)

var defaulttagset = map[string]struct{}{accounts.PUBLIC_TAG: struct{}{}}

func streamtoleafname(ctx context.Context, s *btrdb.Stream) (string, error) {
	tags, err := s.Tags(ctx)
	if err != nil {
		return "", err
	}

	kvs := make([]string, len(tags))
	i := 0
	for key, value := range tags {
		kvs[i] = key + "=" + value
		i++
	}

	return strings.Join(kvs, ","), nil
}

func leafnametotags(leafname string) map[string]string {
	streamtags := make(map[string]string)
	kvs := strings.Split(leafname, ",")
	for _, kv := range kvs {
		strings := strings.SplitN(kv, "=", 2)
		// TODO check if len(strings) == 2 and handle error otherwise
		streamtags[strings[0]] = strings[1]
	}
	return streamtags
}

func getprefixes(ctx context.Context, ec *etcd.Client, ls *LoginSession) (map[string]struct{}, bool, error) {
	var hasall bool
	var tagset map[string]struct{}
	if ls == nil {
		hasall = false
		tagset = defaulttagset
	} else {
		_, hasall = ls.Tags[accounts.ALL_TAG]
		tagset = ls.Tags
	}

	var prefixes map[string]struct{}
	if !hasall {
		prefixes = make(map[string]struct{})
		for tagname := range tagset {
			tagdef, err := accounts.RetrieveTagDef(ctx, ec, tagname)
			if err != nil {
				return nil, false, err
			}

			// Tags that are not defined do not grant any permissions
			if tagdef != nil {
				for pfx := range tagdef.PathPrefix {
					prefixes[pfx] = struct{}{}
				}
			}
		}
	}

	return prefixes, hasall, nil
}

/* Returns a sorted slice of top level elements in the stream tree. */
func treetopPaths(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession) ([]string, error) {
	collections, err := bc.ListAllCollections(ctx)
	if err != nil {
		return nil, err
	}

	prefixes, hasall, err := getprefixes(ctx, ec, ls)
	if err != nil {
		return nil, err
	}

	toplevelset := make(map[string]struct{})
	for _, coll := range collections {
		var toplevel string

		/* Skip this collection if the user doesn't have permission. */
		if !hasall {
			haspermission := false
			for pfx := range prefixes {
				if strings.HasPrefix(coll, pfx) {
					haspermission = true
					break
				}
			}
			if !haspermission {
				continue
			}
		}

		/* Extract the top-level element from the collection name. */
		dotindex := strings.Index(coll, ".")
		if dotindex == -1 {
			toplevel = coll
		} else {
			toplevel = coll[:dotindex]
		}
		toplevelset[toplevel] = struct{}{}
	}

	/* Transfer top-level elements into a slice. */
	treetop := make([]string, len(toplevelset))
	i := 0
	for toplevel := range toplevelset {
		treetop[i] = toplevel
		i++
	}

	sort.Strings(treetop)

	return treetop, nil
}

// func treebranchMetadata(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession, toplevel string) ([]string, error) {
// 	collprefix := toplevel + "."
// 	collections, err := bc.ListCollections(ctx, collprefix)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	prefixes, hasall, err := getprefixes(ctx, ec, ls)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	branches := make([]string, 0, len(collections))
// 	for _, coll := range collections {
// 		/* Skip this collection if the user doesn't have permission. */
// 		if !hasall {
// 			haspermission := false
// 			for pfx := range prefixes {
// 				if strings.HasPrefix(coll, pfx) {
// 					haspermission = true
// 					break
// 				}
// 			}
// 			if !haspermission {
// 				continue
// 			}
// 		}
//
// 		/* Get the streams in the collection. */
// 		streams, err := bc.ListAllStreams(ctx, coll)
// 		if err != nil {
// 			return nil, err
// 		}
//
// 		dotidx := strings.IndexByte(coll, '.')
// 		if dotidx == -1 {
// 			dotidx = len(coll)
// 		}
// 		pathcoll := strings.Replace(coll[dotidx:], ".", "/", -1)
//
// 		for _, stream := range streams {
// 			/* Formulate the path for this stream. */
// 			pathfin, err := streamtoleafname(ctx, stream)
// 			if err != nil {
// 				return nil, err
// 			}
// 			path := pathcoll + "/" + pathfin
//
// 			/* Add path to return slice. */
// 			branches = append(branches, path)
// 		}
// 	}
//
// 	sort.Strings(branches)
//
// 	return branches, nil
// }

func treebranchPaths(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession, toplevel string) ([]string, error) {
	collprefix := toplevel + "."
	collections, err := bc.ListCollections(ctx, collprefix)
	if err != nil {
		return nil, err
	}

	prefixes, hasall, err := getprefixes(ctx, ec, ls)
	if err != nil {
		return nil, err
	}

	branches := make([]string, 0, len(collections))
	for _, coll := range collections {
		/* Skip this collection if the user doesn't have permission. */
		if !hasall {
			haspermission := false
			for pfx := range prefixes {
				if strings.HasPrefix(coll, pfx) {
					haspermission = true
					break
				}
			}
			if !haspermission {
				continue
			}
		}

		dotidx := strings.IndexByte(coll, '.')
		if dotidx == -1 {
			dotidx = len(coll)
		}
		pathcoll := strings.Replace(coll[dotidx:], ".", "/", -1)

		branches = append(branches, pathcoll)
	}

	sort.Strings(branches)

	return branches, nil
}

func treeleafPaths(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession, branchpath string) ([]string, error) {
	coll := strings.Replace(branchpath, "/", ".", -1)

	/* Get the streams in the collection. */
	streams, err := bc.ListAllStreams(ctx, coll)
	if err != nil {
		return nil, err
	}

	leaves := make([]string, 0, len(streams))
	for _, stream := range streams {
		/* Formulate the path for this stream. */
		pathfin, err := streamtoleafname(ctx, stream)
		if err != nil {
			return nil, err
		}
		path := "/" + pathfin

		/* Add path to return slice. */
		leaves = append(leaves, path)
	}

	sort.Strings(leaves)
	return leaves, nil
}

func treeleafMetadata(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession, path string) (map[string]interface{}, error) {
	div := strings.LastIndex(path, "/")
	if div == -1 {
		return nil, errors.New("Invalid path")
	}
	leafname := path[div+1:]
	collection := strings.Replace(path[:div], "/", ".", -1)
	streamtags := leafnametotags(leafname)

	s, err := bc.LookupStream(ctx, collection, streamtags)
	if err != nil {
		return nil, err
	}

	uu := s.UUID()
	return uuidMetadata(ctx, ec, bc, ls, uu)
}

func uuidMetadata(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession, uu uuid.UUID) (map[string]interface{}, error) {
	s := bc.StreamFromUUID(uu)
	ex, err := s.Exists(ctx)
	if err != nil {
		return nil, err
	}
	if !ex {
		return nil, errors.New("Stream does not exist")
	}
	if !hasPermission(ctx, ls, uu) {
		return nil, errors.New("Need permission")
	}

	ann, _, err := s.Annotation(ctx)
	if err != nil {
		return nil, err
	}

	collection, err := s.Collection(ctx)
	if err != nil {
		return nil, err
	}
	pathfin, err := streamtoleafname(ctx, s)
	if err != nil {
		return nil, err
	}

	var doc map[string]interface{}
	err = json.Unmarshal(ann, &doc)
	if err != nil {
		doc = make(map[string]interface{})
	}
	um, ok := doc["UnitofMeasure"]
	if !ok {
		doc["UnitofMeasure"] = "Unknown"
	}
	if _, ok := um.(string); !ok {
		doc["UnitofMeasure"] = "Unknown"
	}
	doc["Path"] = strings.Replace(collection, ".", "/", -1) + "/" + pathfin
	doc["uuid"] = uu.String()

	return doc, nil
}
