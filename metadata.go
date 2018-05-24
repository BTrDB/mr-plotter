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
	"errors"
	"sort"
	"strings"

	"gopkg.in/btrdb.v4"

	acl "github.com/BTrDB/smartgridstore/acl"
	etcd "github.com/coreos/etcd/clientv3"
	"github.com/pborman/uuid"
)

var btrdbSeparator byte = '/'

const plotterSeparator = '/'

func streamtoleafname(ctx context.Context, s *btrdb.Stream) (string, error) {
	var name string
	var ok bool

	ann, _, err := s.CachedAnnotations(ctx)
	if err != nil {
		return "", err
	}

	name, ok = ann["name"]
	if ok {
		return name, nil
	}

	tags, err := s.Tags(ctx)
	if err != nil {
		return "", err
	}

	name, ok = tags["name"]
	if ok {
		return name, nil
	}

	return "$" + s.UUID().String(), nil
}

func leafnametostream(ctx context.Context, bc *btrdb.BTrDB, collection string, leafname string) (*btrdb.Stream, error) {
	if len(leafname) != 0 && leafname[0] == '$' {
		uuidstr := leafname[1:]
		uu := uuid.Parse(uuidstr)
		s := bc.StreamFromUUID(uu)
		ex, err := s.Exists(ctx)
		if err != nil {
			return nil, err
		}
		if !ex {
			return nil, nil
		}
		return s, nil
	}
	matching, err := bc.LookupStreams(ctx, collection, false, nil, map[string]*string{"name": &leafname})
	if err != nil {
		return nil, err
	}
	if len(matching) == 0 {
		matching, err = bc.LookupStreams(ctx, collection, false, map[string]*string{"name": &leafname}, nil)
		if err != nil {
			return nil, err
		}
		if len(matching) == 0 {
			return nil, nil
		}
	}
	return matching[0], nil
}

var publicGroup *acl.Group

func checkPublicGroup() {
	if publicGroup == nil {
		aclEngine := acl.NewACLEngine("btrdb", etcdConn)
		var err error
		publicGroup, err = aclEngine.GetGroup("public")
		if err != nil {
			panic(err)
		}
	}
}
func getprefixes(ctx context.Context, ec *etcd.Client, ls *LoginSession) (map[string]struct{}, error) {
	checkPublicGroup()
	if ls == nil {
		m := make(map[string]struct{})
		for _, p := range publicGroup.Prefixes {
			m[p] = struct{}{}
		}
		return m, nil
	}
	return ls.Prefixes, nil
}

/* Returns a sorted slice of top level elements in the stream tree. */
func treetopPaths(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession) ([]string, error) {
	collections, err := bc.ListAllCollections(ctx)
	if err != nil {
		return nil, err
	}

	prefixes, err := getprefixes(ctx, ec, ls)
	if err != nil {
		return nil, err
	}

	toplevelset := make(map[string]struct{})
	for _, coll := range collections {
		var toplevel string

		/* Skip this collection if the user doesn't have permission. */
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

		/* Extract the top-level element from the collection name. */
		sepindex := strings.Index(coll, string(btrdbSeparator))
		/* If the element starts with the separator, then we would get an empty
		 * toplevel element. To avoid this, split on the next separator. */
		if sepindex == 0 {
			sepindex = strings.Index(coll[1:], string(btrdbSeparator))
			if sepindex != -1 {
				sepindex++
			}
		}
		if sepindex == -1 {
			toplevel = coll
		} else {
			toplevel = coll[:sepindex]
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

func treebranchPaths(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession, toplevel string) ([]string, error) {
	collprefix := toplevel + string(btrdbSeparator)
	collections, err := bc.ListCollections(ctx, collprefix)
	if err != nil {
		return nil, err
	}

	prefixes, err := getprefixes(ctx, ec, ls)
	if err != nil {
		return nil, err
	}

	branches := make([]string, 0, len(collections))
	for _, coll := range collections {
		/* Skip this collection if the user doesn't have permission. */
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

		dotidx := strings.IndexByte(coll, btrdbSeparator)
		if dotidx == -1 {
			dotidx = len(coll)
		}
		pathcoll := strings.Replace(coll[dotidx:], string(btrdbSeparator), string(plotterSeparator), -1)

		branches = append(branches, pathcoll)
	}

	sort.Strings(branches)

	return branches, nil
}

func treeleafPaths(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession, branchpath string) ([]string, error) {
	coll := strings.Replace(branchpath, string(plotterSeparator), string(btrdbSeparator), -1)

	/* Get the streams in the collection. */
	streams, err := bc.LookupStreams(ctx, coll, false, nil, nil)
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
		path := string(plotterSeparator) + pathfin

		/* Add path to return slice. */
		leaves = append(leaves, path)
	}

	sort.Strings(leaves)
	return leaves, nil
}

func treeleafMetadata(ctx context.Context, ec *etcd.Client, bc *btrdb.BTrDB, ls *LoginSession, path string) (map[string]interface{}, error) {
	div := strings.LastIndex(path, string(plotterSeparator))
	if div == -1 {
		return nil, errors.New("Invalid path")
	}
	leafname := path[div+1:]
	collection := strings.Replace(path[:div], string(plotterSeparator), string(btrdbSeparator), -1)
	s, err := leafnametostream(ctx, bc, collection, leafname)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("Stream does not exist")
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

	ann, _, err := s.CachedAnnotations(ctx)
	if err != nil {
		return nil, err
	}

	tags, err := s.Tags(ctx)
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

	var doc = map[string]interface{}{
		"annotations": ann,
		"tags":        tags,
		"path":        strings.Replace(collection, string(btrdbSeparator), string(plotterSeparator), -1) + string(plotterSeparator) + pathfin,
		"uuid":        uu.String(),
	}

	return doc, nil
}
