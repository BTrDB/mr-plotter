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
	"hash"
	"io"
	"fmt"
	"log"
	"time"

	"github.com/SoftwareDefinedBuildings/mr-plotter/accounts"
	etcd "github.com/coreos/etcd/clientv3"
)

var sessionExpirySeconds uint64

var aes_encrypt_cipher cipher.Block
var hmac_key []byte

type LoginSession struct {
	Issued int64
	Tags []string
	User string
}

func setSessionExpiry(seconds uint64) {
	sessionExpirySeconds = seconds
}

func setEncryptKey(key []byte) error {
	var keylen int = len(key)
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

func checkpassword(ctx context.Context, etcdConn *etcd.Client, user string, password []byte) (*accounts.MrPlotterAccount, error) {
	acc, err := accounts.RetrieveAccount(ctx, etcdConn, user)
	if err != nil {
		return nil, err
	}

	correct, err := acc.CheckPassword(password)
	if err != nil {
		return nil, err
	}

	if correct {
		return acc, nil
	} else {
		return nil, nil
	}
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

	taglist := make([]string, len(acc.Tags))
	for tag := range acc.Tags {
		taglist = append(taglist, tag)
	}

	// Create a new session
	loginsession := &LoginSession{
		Issued: time.Now().Unix(),
		Tags: taglist,
		User: user,
	}

	// Construct the JSON plaintext for this login session
	var plaintext []byte
	plaintext, err = json.Marshal(loginsession)
	if err != nil {
		log.Fatalf("Could not JSON-encode login session: %v", err)
	}
	var blocksize int = aes_encrypt_cipher.BlockSize()
	var paddinglen int = blocksize - (len(plaintext) % blocksize)
	var padding []byte = make([]byte, paddinglen)
	plaintext = append(plaintext, padding...)

	// Encrypt and MAC the plaintext to get the token
	// The token consists of the IV, ciphertext, and HMAC concatenated
	var hmac_hash hash.Hash = hmac.New(sha512.New, hmac_key)
	var macsize int = hmac_hash.Size()
	var token []byte = make([]byte, blocksize + len(plaintext), blocksize + len(plaintext) + macsize)
	var iv []byte = token[:blocksize]
	var ciphertext []byte = token[blocksize:]

	_, err = io.ReadFull(rand.Reader, iv)
	if err != nil {
		log.Fatalf("Could not generate IV: %v", err)
	}

	var encrypter cipher.BlockMode = cipher.NewCBCEncrypter(aes_encrypt_cipher, iv)
	encrypter.CryptBlocks(ciphertext, plaintext)

	_, err = hmac_hash.Write(plaintext)
	if err != nil {
		log.Fatalf("Could not compute HMAC of plaintext token: %v", err)
	}
	token = hmac_hash.Sum(token)

	return token, nil
}

func decodetoken(token []byte) []byte {
	var hmac_hash hash.Hash = hmac.New(sha512.New, hmac_key)

	var blocksize int = aes_encrypt_cipher.BlockSize()
	var macsize int = hmac_hash.Size()

	if len(token) <= blocksize + macsize {
		return nil
	}

	var iv []byte = token[:blocksize]
	var ciphertext []byte = token[blocksize:len(token) - macsize]
	var mac []byte = token[len(token) - macsize:]

	if (len(ciphertext) % blocksize) != 0 {
		return nil
	}

	var plaintext []byte = make([]byte, len(ciphertext))
	var decrypter cipher.BlockMode = cipher.NewCBCDecrypter(aes_encrypt_cipher, iv)
	decrypter.CryptBlocks(plaintext, ciphertext)

	_, err := hmac_hash.Write(plaintext)
	if err != nil {
		log.Fatalf("Could not compute HMAC of plaintext token: %v", err)
	}
	var computedmac []byte = hmac_hash.Sum(make([]byte, 0, macsize))

	if !hmac.Equal(computedmac, mac) {
		log.Printf("Invalid MAC detected: someone is trying to forge a token!")
		return nil
	}

	return plaintext
}

func stolenkeys() {
	log.Fatalf("THE MAC KEY HAS BEEN STOLEN, AND THE ENCRYPT KEY PROBABLY TOO. CHANGE THE KEYS AND RESTART THIS PROGRAM.")
}

func getloginsession(token []byte) *LoginSession {
	var plaintext []byte = decodetoken(token)
	if plaintext == nil {
		return nil
	}

	var i int
	for i = len(plaintext) - 1; i >= 0; i-- {
		if plaintext[i] != 0 {
			break
		}
	}

	if len(plaintext) - i - 1 >= aes_encrypt_cipher.BlockSize() {
		log.Println("Invalid padding on token is correctly MAC'ed")
		stolenkeys()
		return nil
	}

	var rawjson []byte = plaintext[:i + 1]
	var loginsession *LoginSession
	var err error = json.Unmarshal(rawjson, &loginsession)
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

	var now int64 = time.Now().Unix()
	if uint64(now - loginsession.Issued) >= sessionExpirySeconds {
		log.Printf("Session expired: (issued at %v, expired at %v, now is %v)", loginsession.Issued, loginsession.Issued + int64(sessionExpirySeconds), now)
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

func usertags(token []byte) []string {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return nil
	}

	return loginsession.Tags
}

func userchangepassword(ctx context.Context, etcdConn *etcd.Client, token []byte, oldpw []byte, newpw []byte) (string) {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return ERROR_INVALID_TOKEN
	}

	acc, err := accounts.RetrieveAccount(ctx, etcdConn, loginsession.User)
	if err != nil {
		return "Server error"
	}

	correct, err := acc.CheckPassword(oldpw)
	if err != nil {
		return "Server error"
	}

	if !correct {
		return "Incorrect password"
	}

	err = acc.SetPassword(newpw)
	if err != nil {
		return "Server error"
	}

	success, err := accounts.UpsertAccountAtomically(ctx, etcdConn, acc)
	if err != nil {
		return "Server error"
	}

	if success {
		return SUCCESS
	} else {
		return "Transaction failed; try again"
	}
}
