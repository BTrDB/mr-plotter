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

/* This file contains the logic for users to login and change their password.
   It also contains the logic to keep track of login sessions. */

package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	acl "github.com/BTrDB/smartgridstore/acl"
	etcd "github.com/coreos/etcd/clientv3"
)

var sessionExpirySeconds uint64

var aes_encrypt_cipher cipher.Block
var hmac_key []byte

type LoginSession struct {
	Issued   int64
	Prefixes map[string]struct{}
	User     string
}

func (loginsession *LoginSession) PrefixSlice() []string {
	prefixlist := make([]string, len(loginsession.Prefixes))
	for pfx := range loginsession.Prefixes {
		prefixlist = append(prefixlist, pfx)
	}
	return prefixlist
}

func setSessionExpiry(seconds uint64) {
	sessionExpirySeconds = seconds
}

func setEncryptKey(key []byte) error {
	var keylen = len(key)
	if keylen != 16 && keylen != 24 && keylen != 32 {
		return fmt.Errorf("Key length is invalid: must be 16, 24, or 32 bytes (got %d bytes)", keylen)
	}
	cipher, err := aes.NewCipher(key)
	if err == nil {
		aes_encrypt_cipher = cipher
	}
	return err
}

func setMACKey(key []byte) error {
	var keylen int = len(key)
	if keylen < 16 {
		return fmt.Errorf("Key length must be at least 16 bytes (got %d bytes)", keylen)
	}
	hmac_key = key
	return nil
}

func checkpassword(ctx context.Context, etcdConn *etcd.Client, user string, password []byte) (*acl.User, error) {
	ae := acl.NewACLEngine("btrdb", etcdConn)
	okay, u, err := ae.AuthenticateUser(user, string(password))
	if err != nil {
		return nil, err
	}
	if !okay {
		return nil, nil
	}
	if u.HasCapability("plotter") {
		return u, nil
	}
	return nil, nil
}

/* Writing to the returned slice results in undefined behavior.
   The returned slice is guaranteed to have a length of TOKEN_BYTE_LEN. */
func userlogin(ctx context.Context, etcdConn *etcd.Client, user string, password []byte) ([]byte, error) {
	acc, err := checkpassword(ctx, etcdConn, user, password)
	if err != nil {
		return nil, err
	} else if acc == nil {
		/* Wrong password */
		return nil, nil
	}

	// Create a new session
	loginsession := &LoginSession{
		Issued:   time.Now().Unix(),
		Prefixes: make(map[string]struct{}),
		User:     user,
	}
	for _, g := range acc.FullGroups {
		for _, cap := range g.Capabilities {
			if cap == "plotter" {
				for _, p := range g.Prefixes {
					loginsession.Prefixes[p] = struct{}{}
				}
			}
		}
	}

	// Construct the JSON plaintext for this login session
	var plaintext []byte
	plaintext, err = json.Marshal(loginsession)
	if err != nil {
		log.Fatalf("Could not JSON-encode login session: %v", err)
	}
	var blocksize = aes_encrypt_cipher.BlockSize()
	var paddinglen = (blocksize - (len(plaintext) % blocksize)) % blocksize
	var padding = make([]byte, paddinglen)
	plaintext = append(plaintext, padding...)

	// Encrypt and MAC the plaintext to get the token
	// The token consists of the IV, ciphertext, and HMAC concatenated
	var hmac_hash = hmac.New(sha512.New, hmac_key)
	var macsize = hmac_hash.Size()
	var token = make([]byte, blocksize+len(plaintext), blocksize+len(plaintext)+macsize)
	var iv = token[:blocksize]
	var ciphertext = token[blocksize:]

	_, err = io.ReadFull(rand.Reader, iv)
	if err != nil {
		log.Fatalf("Could not generate IV: %v", err)
	}

	var encrypter = cipher.NewCBCEncrypter(aes_encrypt_cipher, iv)
	encrypter.CryptBlocks(ciphertext, plaintext)

	_, err = hmac_hash.Write(plaintext)
	if err != nil {
		log.Fatalf("Could not compute HMAC of plaintext token: %v", err)
	}
	token = hmac_hash.Sum(token)

	return token, nil
}

func decodetoken(token []byte) []byte {
	var hmac_hash = hmac.New(sha512.New, hmac_key)

	var blocksize = aes_encrypt_cipher.BlockSize()
	var macsize = hmac_hash.Size()

	if len(token) <= blocksize+macsize {
		return nil
	}

	var iv = token[:blocksize]
	var ciphertext = token[blocksize : len(token)-macsize]
	var mac = token[len(token)-macsize:]

	if (len(ciphertext) % blocksize) != 0 {
		return nil
	}

	var plaintext = make([]byte, len(ciphertext))
	var decrypter = cipher.NewCBCDecrypter(aes_encrypt_cipher, iv)
	decrypter.CryptBlocks(plaintext, ciphertext)

	_, err := hmac_hash.Write(plaintext)
	if err != nil {
		log.Fatalf("Could not compute HMAC of plaintext token: %v", err)
	}
	var computedmac = hmac_hash.Sum(make([]byte, 0, macsize))

	if !hmac.Equal(computedmac, mac) {
		log.Printf("Invalid MAC detected: someone is trying to forge a token!")
		return nil
	}

	return plaintext
}

func stolenkeys() {
	log.Println("THE MAC KEY HAS BEEN STOLEN, AND THE ENCRYPT KEY PROBABLY TOO. CHANGE THE KEYS AND RESTART THIS PROGRAM.")
}

func getloginsession(token []byte) *LoginSession {
	var plaintext = decodetoken(token)
	if plaintext == nil {
		return nil
	}

	var i int
	for i = len(plaintext) - 1; i >= 0; i-- {
		if plaintext[i] != 0 {
			break
		}
	}

	if len(plaintext)-i-1 >= aes_encrypt_cipher.BlockSize() {
		log.Println("Invalid padding on token is correctly MAC'ed")
		stolenkeys()
		return nil
	}

	var rawjson = plaintext[:i+1]
	var loginsession *LoginSession
	var err = json.Unmarshal(rawjson, &loginsession)
	if err != nil {
		log.Printf("Correctly MAC'ed token is incorrect JSON: %v", err)
		stolenkeys()
		return nil
	}
	if loginsession == nil {
		log.Println("Correctly MAC'ed token is null")
		stolenkeys()
		return nil
	}

	var now = time.Now().Unix()
	if uint64(now-loginsession.Issued) >= sessionExpirySeconds {
		log.Printf("Session expired: (issued at %v, expired at %v, now is %v)", loginsession.Issued, loginsession.Issued+int64(sessionExpirySeconds), now)
		return nil
	}

	return loginsession
}

func userlogoff(token []byte) bool {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return false
	}

	return true
}

func userprefixes(token []byte) []string {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return nil
	}

	return loginsession.PrefixSlice()
}

// func userchangepassword(ctx context.Context, etcdConn *etcd.Client, token []byte, oldpw []byte, newpw []byte) string {
// 	loginsession := getloginsession(token)
// 	if loginsession == nil {
// 		return ERROR_INVALID_TOKEN
// 	}
//
// 	acc, err := accounts.RetrieveAccount(ctx, etcdConn, loginsession.User)
// 	if err != nil {
// 		return "Server error"
// 	}
//
// 	correct, err := acc.CheckPassword(oldpw)
// 	if err != nil {
// 		return "Server error"
// 	}
//
// 	if !correct {
// 		return "Incorrect password"
// 	}
//
// 	err = acc.SetPassword(newpw)
// 	if err != nil {
// 		return "Server error"
// 	}
//
// 	success, err := accounts.UpsertAccountAtomically(ctx, etcdConn, acc)
// 	if err != nil {
// 		return "Server error"
// 	}
//
// 	if success {
// 		return SUCCESS
// 	} else {
// 		return "Transaction failed; try again"
// 	}
// }
