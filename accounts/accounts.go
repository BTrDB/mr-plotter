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

// Package accounts implements tools to manage account information for Mr. Plotter.
// Account information is stored in etcd, and so a Version 3 etcd client is needed
// for most of the API functions.
package accounts

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	etcd "github.com/coreos/etcd/clientv3"
	"github.com/samkumar/etcdstruct"
	"github.com/ugorji/go/codec"
)

// The "public" tag is used to specify streams viewable to all users, even those
// who are not logged in.
const PUBLIC_TAG = "public"

// The "all" tag represents permissions to view all streams. It is stored
// implicitly (i.e. not in etcd) and cannot be redefined.
const ALL_TAG = "all"

const accountpath string = "mrplotter/accounts/"
const tagpath string = "mrplotter/tagdefs/"

var etcdprefix string = ""

var mp codec.Handle = &codec.MsgpackHandle{}

// MrPlotterAccount abstracts account information for a single user.
type MrPlotterAccount struct {
	Username     string
	Tags         map[string]struct{}
	PasswordHash []byte

	retrievedRevision int64
}

// MrPlotterTagDef abstracts a mapping from a Tag to set of collection prefixes.
type MrPlotterTagDef struct {
	Tag        string
	PathPrefix map[string]struct{}

	retrievedRevision int64
}

func (acc *MrPlotterAccount) SetRetrievedRevision(rev int64) {
	acc.retrievedRevision = rev
}

func (acc *MrPlotterAccount) GetRetrievedRevision() int64 {
	return acc.retrievedRevision
}

// Sets the password for a user.
func (acc *MrPlotterAccount) SetPassword(newPassword []byte) error {
	phash, err := bcrypt.GenerateFromPassword(newPassword, bcrypt.DefaultCost)
	if err == nil {
		acc.PasswordHash = phash
	}
	return err
}

// Checks whether the provided password matches the user's password. Returns
// true if the provided password is correct, and false if it is not. If the
// returned error is not nil, then the returned boolean should be ignored.
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

func (tdef *MrPlotterTagDef) SetRetrievedRevision(rev int64) {
	tdef.retrievedRevision = rev
}

func (tdef *MrPlotterTagDef) GetRetrievedRevision() int64 {
	return tdef.retrievedRevision
}

// Sets the prefix added to keys in the etcd database.
// The keys used are of the form <prefix>mrplotter/accounts/<username>.
// The prefix allows multiple configurations for Mr. Plotter to coexist in a
// single etcd database system.
//
// Modifying the prefix while an operation is in progress results in undefined
// behavior. Furthermore, MrPlotterAccount structs returned by RetrieveAccount
// or RetrieveMultipleAccounts must always be used with the same prefix with
// which they were generated.
func SetEtcdKeyPrefix(prefix string) {
	etcdprefix = prefix
}

// Gets the base path for account data in etcd.
func GetAccountEtcdPath() string {
	return fmt.Sprintf("%s%s", etcdprefix, accountpath)
}

// Gets the base path for tag definitions in etcd.
func GetTagEtcdPath() string {
	return fmt.Sprintf("%s%s", etcdprefix, tagpath)
}

func getEtcdKey(name string, typepath string) string {
	return fmt.Sprintf("%s%s%s", etcdprefix, typepath, name)
}

func getNameFromEtcdKey(etcdKey string, typepath string) string {
	return etcdKey[len(etcdprefix)+len(typepath):]
}

// Retrieves the account information for the specified user.
// Setting the "Username" field in the returned struct renders it unsuitable
// for use with the "UpsertAccountAtomically" function.
func RetrieveAccount(ctx context.Context, etcdClient *etcd.Client, username string) (acc *MrPlotterAccount, err error) {
	acc = &MrPlotterAccount{}
	exists, err := etcdstruct.RetrieveEtcdStruct(ctx, etcdClient, getEtcdKey(username, accountpath), acc)
	if !exists {
		acc = nil
	}
	return
}

// Retrieves the tag definition for the specified tag.
// Setting the "Tag" field in the returned struct renders it unsuitable
// for use with the "UpsertTagDefAtomically" function.
func RetrieveTagDef(ctx context.Context, etcdClient *etcd.Client, tag string) (tdef *MrPlotterTagDef, err error) {
	tdef = &MrPlotterTagDef{}
	exists, err := etcdstruct.RetrieveEtcdStruct(ctx, etcdClient, getEtcdKey(tag, tagpath), tdef)
	if !exists {
		tdef = nil
	}
	return
}

// Updates the account according to the provided account information, creating
// a new user account if a user with that username does not exist.
func UpsertAccount(ctx context.Context, etcdClient *etcd.Client, acc *MrPlotterAccount) error {
	return etcdstruct.UpsertEtcdStruct(ctx, etcdClient, getEtcdKey(acc.Username, accountpath), acc)
}

// Same as UpsertAccount, but for Tag Definitions
func UpsertTagDef(ctx context.Context, etcdClient *etcd.Client, tdef *MrPlotterTagDef) error {
	return etcdstruct.UpsertEtcdStruct(ctx, etcdClient, getEtcdKey(tdef.Tag, tagpath), tdef)
}

// Same as UpsertAccount, but fails if the account was updated meanwhile. This
// allows one to ensure that the read-modify-write operation required to update
// account information can be done atomically. Returns true if the operation
// succeeds, and returns false otherwise.
//
// The rules are as follows: if the MrPlotterAccount struct was created
// directly, the operation fails if the account already exists in etcd. If the
// MrPlotterAccount struct was returned by RetrieveAccount or
// RetrieveMultipleAccounts, the operation fails if the data stored in etcd
// was updated after the account information was retrieved. You should not
// modify the "Username" field of a struct returned by RetrieveAccount or
// RetrieveMultipleAccounts and then use it with this function. Setting the
// "Username" field of a struct to be used with this function is only allowed
// for structs that are created directly.
func UpsertAccountAtomically(ctx context.Context, etcdClient *etcd.Client, acc *MrPlotterAccount) (bool, error) {
	return etcdstruct.UpsertEtcdStructAtomic(ctx, etcdClient, getEtcdKey(acc.Username, accountpath), acc)
}

// Same as UpsertAccountAtomically, but for Tag Definitions.
func UpsertTagDefAtomically(ctx context.Context, etcdClient *etcd.Client, tdef *MrPlotterTagDef) (bool, error) {
	return etcdstruct.UpsertEtcdStructAtomic(ctx, etcdClient, getEtcdKey(tdef.Tag, tagpath), tdef)
}

// Deletes the account of the user with the provided username.
func DeleteAccount(ctx context.Context, etcdClient *etcd.Client, username string) error {
	_, err := etcdstruct.DeleteEtcdStructs(ctx, etcdClient, getEtcdKey(username, accountpath))
	return err
}

// Deletes the tag definition with the provided name
func DeleteTagDef(ctx context.Context, etcdClient *etcd.Client, tag string) error {
	_, err := etcdstruct.DeleteEtcdStructs(ctx, etcdClient, getEtcdKey(tag, tagpath))
	return err
}

// Retrieves the account information of all users whose username begins with
// the provided prefix.
// If one entry is in a corrupt state and cannot be decoded, its Tags set will
// be set to nil and decoding will continue.
// Setting the "Username" field in the returned struct renders it unsuitable
// for use with the "UpsertAccountAtomically" function.
func RetrieveMultipleAccounts(ctx context.Context, etcdClient *etcd.Client, usernameprefix string) ([]*MrPlotterAccount, error) {
	etcdKeyPrefix := getEtcdKey(usernameprefix, accountpath)
	accs := make([]*MrPlotterAccount, 0, 16)
	err := etcdstruct.RetrieveEtcdStructs(ctx, etcdClient, func(key []byte) etcdstruct.EtcdStruct {
		acc := &MrPlotterAccount{}
		accs = append(accs, acc)
		return acc
	}, func(es etcdstruct.EtcdStruct, key []byte) {
		acc := es.(*MrPlotterAccount)
		acc.Username = getNameFromEtcdKey(string(key), tagpath)
		acc.Tags = nil
	}, etcdKeyPrefix, etcd.WithPrefix())
	if err != nil {
		return nil, err
	}

	return accs, err
}

// Retrieves the account information of all users whose username begins with
// the provided prefix.
// If one entry is in a corrupt state and cannot be decoded, its Tags set will
// be set to nil and decoding will continue.
// Setting the "Username" field in the returned struct renders it unsuitable
// for use with the "UpsertAccountAtomically" function.
func RetrieveMultipleTagDefs(ctx context.Context, etcdClient *etcd.Client, tagprefix string) ([]*MrPlotterTagDef, error) {
	etcdKeyPrefix := getEtcdKey(tagprefix, tagpath)
	tdefs := make([]*MrPlotterTagDef, 0, 16)
	err := etcdstruct.RetrieveEtcdStructs(ctx, etcdClient, func(key []byte) etcdstruct.EtcdStruct {
		tdef := &MrPlotterTagDef{}
		tdefs = append(tdefs, tdef)
		return tdef
	}, func(es etcdstruct.EtcdStruct, key []byte) {
		tdef := es.(*MrPlotterTagDef)
		tdef.Tag = getNameFromEtcdKey(string(key), tagpath)
		tdef.PathPrefix = nil
	}, etcdKeyPrefix, etcd.WithPrefix())
	if err != nil {
		return nil, err
	}

	return tdefs, err
}

// Deletes the accounts of all users whose username begins with the provided
// prefix.
func DeleteMultipleAccounts(ctx context.Context, etcdClient *etcd.Client, usernameprefix string) (int64, error) {
	return etcdstruct.DeleteEtcdStructs(ctx, etcdClient, getEtcdKey(usernameprefix, accountpath), etcd.WithPrefix())
}

// Deletes all tag definitions beginning with the given prefix.
func DeleteMultipleTagDefs(ctx context.Context, etcdClient *etcd.Client, tagprefix string) (int64, error) {
	return etcdstruct.DeleteEtcdStructs(ctx, etcdClient, getEtcdKey(tagprefix, tagpath), etcd.WithPrefix())
}
