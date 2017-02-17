/*
 * Copyright (c) 2017 Sam Kumar <samkumar@berkeley.edu>
 * Copyright (c) 2017 University of California, Berkeley
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are met:
 *     * Redistributions of source code must retain the above copyright
 *       notice, this list of conditions and the following disclaimer.
 *     * Redistributions in binary form must reproduce the above copyright
 *       notice, this list of conditions and the following disclaimer in the
 *       documentation and/or other materials provided with the distribution.
 *     * Neither the name of the University of California, Berkeley nor the
 *       names of its contributors may be used to endorse or promote products
 *       derived from this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
 * ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 * WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 * DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNERS OR CONTRIBUTORS BE LIABLE FOR
 * ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
 * (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
 * LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
 * ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
 * SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

package permalink

import (
	"context"
	"fmt"

	etcd "github.com/coreos/etcd/clientv3"
)

const etcdpath string = "mrplotter/permalinks/"

var etcdprefix = ""

func getPermalinkEtcdKey(plnk string) string {
	return fmt.Sprintf("%s%s%s", etcdprefix, etcdpath, plnk)
}

// Sets the prefix added to keys in the etcd database.
// The keys used are of the form <prefix>mrplotter/permalinks/<id>.
// The prefix allows separate deployments of Mr. Plotter to coexist in a
// single etcd database system.
func SetEtcdKeyPrefix(prefix string) {
	etcdprefix = prefix
}

// Retrieves the data for a permalink, given its ID.
func RetrievePermalinkData(ctx context.Context, etcdClient *etcd.Client, id string) ([]byte, error) {
	etcdKey := getPermalinkEtcdKey(id)
	presp, err := etcdClient.Get(ctx, etcdKey)
	if err != nil {
		return nil, err
	}

	if len(presp.Kvs) == 0 {
		return nil, nil
	} else {
		return presp.Kvs[0].Value, nil
	}
}

// Updates the data for a permalink, creating it if it does not exist.
func UpsertPermalinkData(ctx context.Context, etcdClient *etcd.Client, id string, data []byte) error {
	etcdKey := getPermalinkEtcdKey(id)
	_, err := etcdClient.Put(ctx, etcdKey, string(data))
	return err
}

// Same as UpdatePermalinkData, but fails if the permalink already exists in
// the database. Returns true on success and false on failure.
func InsertPermalinkData(ctx context.Context, etcdClient *etcd.Client, id string, data []byte) (bool, error) {
	etcdKey := getPermalinkEtcdKey(id)
	resp, err := etcdClient.Txn(ctx).
		If(etcd.Compare(etcd.ModRevision(etcdKey), "=", 0)).
		Then(etcd.OpPut(etcdKey, string(data))).
		Commit()

	if resp != nil {
		return resp.Succeeded, err
	} else {
		return false, err
	}
}
