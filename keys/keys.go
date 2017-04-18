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

// Package keys implements tools to manage HTTPS certificates and session keys
// for Mr. Plotter. The certificates are stored in etcd, so a Version 3 etcd
// client is needed for most of the API functions.
package keys

import (
	"context"
	"fmt"

	"golang.org/x/crypto/acme/autocert"

	etcd "github.com/coreos/etcd/clientv3"
	"github.com/samkumar/etcdstruct"
)

var etcdprefix = ""

const etcdpath = "mrplotter/keys/"

const cachesuffix = "autocert_cache/"

type HardcodedTLSCertificate struct {
	Cert []byte
	Key  []byte

	retrievedRevision int64
}

func (h *HardcodedTLSCertificate) SetRetrievedRevision(rev int64) {
	h.retrievedRevision = rev
}
func (h *HardcodedTLSCertificate) GetRetrievedRevision() int64 {
	return h.retrievedRevision
}

type SessionKeys struct {
	EncryptKey []byte
	MACKey     []byte

	retrievedRevision int64
}

func (sk *SessionKeys) SetRetrievedRevision(rev int64) {
	sk.retrievedRevision = rev
}
func (sk *SessionKeys) GetRetrievedRevision() int64 {
	return sk.retrievedRevision
}

// Gets the base path for https certificates in etcd.
func GetHttpsCertEtcdPath() string {
	return fmt.Sprintf("%s%s", etcdprefix, etcdpath)
}

func SetEtcdKeyPrefix(prefix string) {
	etcdprefix = prefix
}

func getHttpsCertEtcdKey(key string) string {
	return fmt.Sprintf("%s%s%s", etcdprefix, etcdpath, key)
}

func putHttpsCertKey(ctx context.Context, etcdClient *etcd.Client, key string, value string) error {
	etcdKey := getHttpsCertEtcdKey(key)
	_, err := etcdClient.Put(ctx, etcdKey, value)
	return err
}

func getHttpsCertKey(ctx context.Context, etcdClient *etcd.Client, key string) (string, error) {
	etcdKey := getHttpsCertEtcdKey(key)
	hresp, err := etcdClient.Get(ctx, etcdKey)
	if err != nil {
		return "", err
	}

	if len(hresp.Kvs) == 0 {
		return "", nil
	}

	return string(hresp.Kvs[0].Value), nil
}

func delHttpsCertKey(ctx context.Context, etcdClient *etcd.Client, key string, isPrefix bool) error {
	var err error
	etcdKey := getHttpsCertEtcdKey(key)

	if isPrefix {
		_, err = etcdClient.Delete(ctx, etcdKey, etcd.WithPrefix())
	} else {
		_, err = etcdClient.Delete(ctx, etcdKey)
	}

	return err
}

func SetCertificateSource(ctx context.Context, etcdClient *etcd.Client, source string) error {
	return putHttpsCertKey(ctx, etcdClient, "source", source)
}

func GetCertificateSource(ctx context.Context, etcdClient *etcd.Client) (string, error) {
	return getHttpsCertKey(ctx, etcdClient, "source")
}

func SetAutocertHostname(ctx context.Context, etcdClient *etcd.Client, hostname string) error {
	return putHttpsCertKey(ctx, etcdClient, "hostname", hostname)
}

func GetAutocertHostname(ctx context.Context, etcdClient *etcd.Client) (string, error) {
	return getHttpsCertKey(ctx, etcdClient, "hostname")
}

func SetAutocertEmail(ctx context.Context, etcdClient *etcd.Client, email string) error {
	return putHttpsCertKey(ctx, etcdClient, "email", email)
}

func GetAutocertEmail(ctx context.Context, etcdClient *etcd.Client) (string, error) {
	return getHttpsCertKey(ctx, etcdClient, "email")
}

func getAutocertCacheKey(key string) string {
	return fmt.Sprintf("%s%s", cachesuffix, key)
}

func GetAutocertCache(ctx context.Context, etcdClient *etcd.Client, key string) (string, error) {
	autocertKey := getAutocertCacheKey(key)
	return getHttpsCertKey(ctx, etcdClient, autocertKey)
}

func PutAutocertCache(ctx context.Context, etcdClient *etcd.Client, key string, val string) error {
	autocertKey := getAutocertCacheKey(key)
	return putHttpsCertKey(ctx, etcdClient, autocertKey, val)
}

func DeleteAutocertCache(ctx context.Context, etcdClient *etcd.Client, key string) error {
	autocertKey := getAutocertCacheKey(key)
	return delHttpsCertKey(ctx, etcdClient, autocertKey, false)
}

func DropAutocertCache(ctx context.Context, etcdClient *etcd.Client) error {
	autocertPrefix := getAutocertCacheKey("")
	return delHttpsCertKey(ctx, etcdClient, autocertPrefix, true)
}

func RetrieveHardcodedTLSCertificate(ctx context.Context, etcdClient *etcd.Client) (h *HardcodedTLSCertificate, err error) {
	h = &HardcodedTLSCertificate{}
	exists, err := etcdstruct.RetrieveEtcdStruct(ctx, etcdClient, getHttpsCertEtcdKey("hardcoded"), h)
	if !exists {
		h = nil
	}
	return
}

func UpsertHardcodedTLSCertificate(ctx context.Context, etcdClient *etcd.Client, hardcoded *HardcodedTLSCertificate) error {
	return etcdstruct.UpsertEtcdStruct(ctx, etcdClient, getHttpsCertEtcdKey("hardcoded"), hardcoded)
}

func UpsertHardcodedTLSCertificateAtomically(ctx context.Context, etcdClient *etcd.Client, hardcoded *HardcodedTLSCertificate) (bool, error) {
	return etcdstruct.UpsertEtcdStructAtomic(ctx, etcdClient, getHttpsCertEtcdKey("hardcoded"), hardcoded)
}

func RetrieveSessionKeys(ctx context.Context, etcdClient *etcd.Client) (sk *SessionKeys, err error) {
	sk = &SessionKeys{}
	exists, err := etcdstruct.RetrieveEtcdStruct(ctx, etcdClient, getHttpsCertEtcdKey("session"), sk)
	if !exists {
		sk = nil
	}
	return
}

func UpsertSessionKeys(ctx context.Context, etcdClient *etcd.Client, sk *SessionKeys) error {
	return etcdstruct.UpsertEtcdStruct(ctx, etcdClient, getHttpsCertEtcdKey("session"), sk)
}

func UpsertSessionKeysAtomically(ctx context.Context, etcdClient *etcd.Client, sk *SessionKeys) (bool, error) {
	return etcdstruct.UpsertEtcdStructAtomic(ctx, etcdClient, getHttpsCertEtcdKey("session"), sk)
}

type EtcdCache struct {
	etcdClient *etcd.Client
}

func NewEtcdCache(etcdClient *etcd.Client) *EtcdCache {
	return &EtcdCache{
		etcdClient: etcdClient,
	}
}

func (ec *EtcdCache) Get(ctx context.Context, key string) ([]byte, error) {
	strval, err := GetAutocertCache(ctx, ec.etcdClient, key)
	if strval == "" && err == nil {
		return nil, autocert.ErrCacheMiss
	}

	return []byte(strval), err
}

func (ec *EtcdCache) Put(ctx context.Context, key string, val []byte) error {
	strval := string(val)
	return PutAutocertCache(ctx, ec.etcdClient, key, strval)
}

func (ec *EtcdCache) Delete(ctx context.Context, key string) error {
	return DeleteAutocertCache(ctx, ec.etcdClient, key)
}
