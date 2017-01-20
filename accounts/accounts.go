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

package accounts

import (
    "context"
    "fmt"

    "golang.org/x/crypto/bcrypt"

    "github.com/ugorji/go/codec"
    etcd "github.com/coreos/etcd/clientv3"
)

const etcdpath string = "mrplotter/accounts/"
var etcdprefix = ""

var mp codec.Handle = &codec.MsgpackHandle{}

type MrPlotterAccount struct {
    Username string
    Tags map[string]struct{}
    PasswordHash []byte

    retrievedRevision int64
}

func (acc *MrPlotterAccount) SetPassword(newPassword []byte) error {
    phash, err := bcrypt.GenerateFromPassword(newPassword, bcrypt.DefaultCost)
    if (err == nil) {
        acc.PasswordHash = phash
    }
    return err
}

func (acc *MrPlotterAccount) CheckPassword(password []byte) (bool, error) {
    err := bcrypt.CompareHashAndPassword(acc.PasswordHash, password)
    if err == nil {
        return true, nil
    } else if err == bcrypt.ErrMismatchedHashAndPassword {
        return false, nil
    } else {
        return false, err
    }
}

func encodeAccount(acc *MrPlotterAccount) ([]byte, error) {
    var encoded []byte

    encoder := codec.NewEncoderBytes(&encoded, mp)
    err := encoder.Encode(acc)
    if err != nil {
        return nil, err
    }

    return encoded, nil
}

func decodeAccount(encoded []byte) (*MrPlotterAccount, error) {
    var acc *MrPlotterAccount

    decoder := codec.NewDecoderBytes(encoded, mp)
    err := decoder.Decode(&acc)
    if err != nil {
        return nil, err
    }

    return acc, nil
}

func SetEtcdKeyPrefix(prefix string) {
    etcdprefix = prefix;
}

func getEtcdKey(username string) string {
    return fmt.Sprintf("%s%s%s", etcdprefix, etcdpath, username)
}

func getUsernameFromEtcdKey(etcdKey string) string {
    return etcdKey[len(etcdprefix) + len(etcdpath):]
}

func RetrieveAccount(ctx context.Context, etcdClient *etcd.Client, username string) (*MrPlotterAccount, error) {
    etcdKey := getEtcdKey(username)
    resp, err := etcdClient.Get(ctx, etcdKey)
    if err != nil {
        return nil, err
    }

    /* No account with that username exists. */
    if len(resp.Kvs) == 0 {
        return nil, nil
    }

    acc, err := decodeAccount(resp.Kvs[0].Value)
    if err != nil {
        return nil, err
    }

    acc.retrievedRevision = resp.Kvs[0].ModRevision
    return acc, nil
}

func UpsertAccount(ctx context.Context, etcdClient *etcd.Client, acc *MrPlotterAccount) error {
    encoded, err := encodeAccount(acc)
    if err != nil {
        return err
    }

    etcdKey := getEtcdKey(acc.Username)
    _, err = etcdClient.Put(ctx, etcdKey, string(encoded))
    return err
}

func UpsertAccountAtomically(ctx context.Context, etcdClient *etcd.Client, acc *MrPlotterAccount) (bool, error) {
    encoded, err := encodeAccount(acc)
    if err != nil {
        return false, err
    }

    etcdKey := getEtcdKey(acc.Username)
    resp, err := etcdClient.Txn(ctx).
        If(etcd.Compare(etcd.ModRevision(etcdKey), "=", acc.retrievedRevision)).
        Then(etcd.OpPut(etcdKey, string(encoded))).
        Commit()
    if resp != nil {
        return resp.Succeeded, err
    } else {
        return false, err
    }
}

func DeleteAccount(ctx context.Context, etcdClient *etcd.Client, username string) error {
    etcdKey := getEtcdKey(username)
    _, err := etcdClient.Delete(ctx, etcdKey)
    return err
}

func RetrieveMultipleAccounts(ctx context.Context, etcdClient *etcd.Client, usernameprefix string) ([]*MrPlotterAccount, error) {
    etcdKeyPrefix := getEtcdKey(usernameprefix)
    resp, err := etcdClient.Get(ctx, etcdKeyPrefix, etcd.WithPrefix())
    if err != nil {
        return nil, err
    }

    accs := make([]*MrPlotterAccount, 0, len(resp.Kvs))
    for _, kv := range resp.Kvs {
        acc, err := decodeAccount(kv.Value)
        if err != nil {
            acc = &MrPlotterAccount{Username: getUsernameFromEtcdKey(string(kv.Key))}
        }
        acc.retrievedRevision = kv.ModRevision
        accs = append(accs, acc)
    }

    return accs, nil
}

func DeleteMultipleAccounts(ctx context.Context, etcdClient *etcd.Client, usernameprefix string) (int64, error) {
    etcdKeyPrefix := getEtcdKey(usernameprefix)
    resp, err := etcdClient.Delete(ctx, etcdKeyPrefix, etcd.WithPrefix())
    if resp != nil {
        return resp.Deleted, err
    } else {
        return 0, err
    }
}
