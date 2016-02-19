/* Caches streams that users are permitted to see. */

package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	
	uuid "github.com/pborman/uuid"
)

// Maps TAG to a map from UUID to a boolean which represents whether a user with TAG can view the stream corresponding to UUID
var permcache map[string]map[uuid.Array]bool = make(map[string]map[uuid.Array]bool)
var defaulttags = []string{ "public" }

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
	validuuids, ok := permcache[tag]
	if !ok {
		/* Cache Miss: never seen this token */
		validuuids = make(map[uuid.Array]bool)
		permcache[tag] = validuuids
	} else {
		hasPerm, ok = validuuids[uuidBytes.Array()]
		if ok {
			/* Cache Hit */
			return hasPerm
		}
		/* Cache Miss: never seen this UUID */
	}
	
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
	validuuids[uuidarr] = hasPerm
	return hasPerm
}
