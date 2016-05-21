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
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
	
	"golang.org/x/crypto/bcrypt"
	
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

/* 16 bytes should be longer than anyone can guess. */
const TOKEN_BYTE_LEN = 16
var MAX_TOKEN_BYTES = []byte{ 1,
							  0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000,
							  0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000 }

type LoginSession struct {
	lastUsed int64
	user string
	token [TOKEN_BYTE_LEN]byte
	tags []string
}

// Monotonically increasing identifier
var useid uint64

/* Initialized to the above byte value on use. */
var MAX_TOKEN big.Int = big.Int{}

// Maps ID to session
var sessionsbyid map[[TOKEN_BYTE_LEN]byte]*LoginSession = make(map[[TOKEN_BYTE_LEN]byte]*LoginSession)
var sessionsbyidlock sync.Mutex = sync.Mutex{} // also protects session contents
// Maps user to session
var sessionsbyuser map[string]*LoginSession = make(map[string]*LoginSession)
var sessionsbyuserlock sync.RWMutex = sync.RWMutex{}

func checkpassword(passwordConn *mgo.Collection, user string, password []byte) (userdoc map[string]interface{}, err error) {
	userquery := bson.M{ "user": user }
	err = passwordConn.Find(userquery).One(&userdoc)
	if err != nil {
		return
	}
	hashedpasswordbinary := userdoc["password"].(bson.Binary)
	hashedpassword := hashedpasswordbinary.Data
	
	err = bcrypt.CompareHashAndPassword(hashedpassword, password)
	return
}

/* Writing to the returned slice results in undefined behavior.
   The returned slice is guaranteed to have a length of TOKEN_BYTE_LEN. */
func userlogin(passwordConn *mgo.Collection, user string, password []byte) []byte {
	var userdoc map[string]interface{}
	var loginsession *LoginSession
	var err error
	
	userdoc, err = checkpassword(passwordConn, user, password)
	if err != nil {
		return nil
	}
	
	tagintlist, ok := userdoc["tags"].([]interface{})
	if !ok {
		fmt.Println("Corrupt Mongo document: required key \"tags\" does not refer to an object")
		return nil
	}
	
	taglist := make([]string, len(tagintlist))
	for i := 0; i < len(taglist); i++ {
		taglist[i], ok = tagintlist[i].(string)
		if !ok {
			fmt.Printf("Corrupt Mongo document: tag at index %d for user %s is not a string\n", i, user)
			return nil
		}
	}
	
	// Check if we already have a session for this user
	sessionsbyuserlock.RLock()
	loginsession = sessionsbyuser[user]
	sessionsbyuserlock.RUnlock()
	if loginsession == nil {
		// Need to create a new session
		if MAX_TOKEN.Sign() == 0 {
			(&MAX_TOKEN).SetBytes(MAX_TOKEN_BYTES)
		}
		var token *big.Int
		var tokenarr [TOKEN_BYTE_LEN]byte
		for true {
			token, err = rand.Int(rand.Reader, &MAX_TOKEN)
			if err != nil {
				fmt.Println("Could not generate session key")
				return nil
			}
			tokenbytes := token.Bytes()
		
			copy(tokenarr[TOKEN_BYTE_LEN - len(tokenbytes):], tokenbytes)
			
			sessionsbyidlock.Lock()
			if sessionsbyid[tokenarr] == nil {
				break
			} else {
				sessionsbyidlock.Unlock()
			}
		}
		
		loginsession = &LoginSession{
			lastUsed: time.Now().Unix(),
			user: user,
			token: tokenarr,
			tags: taglist,
		}
		
		sessionsbyid[tokenarr] = loginsession
		sessionsbyidlock.Unlock()
		
		sessionsbyuserlock.Lock()
		sessionsbyuser[user] = loginsession
		sessionsbyuserlock.Unlock()
		
		return tokenarr[:]
	} else {
		return loginsession.token[:]
	}
}

func getloginsession(token []byte) *LoginSession {
	var tokenarr [TOKEN_BYTE_LEN]byte
	copy(tokenarr[:], token)
	
	var loginsession *LoginSession
	var now int64 = time.Now().Unix()
	sessionsbyidlock.Lock()
	loginsession = sessionsbyid[tokenarr]
	if loginsession != nil {
		loginsession.lastUsed = now
	}
	sessionsbyidlock.Unlock()
	return loginsession
}

func userlogoff(token []byte) bool {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return false
	}
	
	sessionsbyidlock.Lock()
	delete(sessionsbyid, loginsession.token)
	sessionsbyidlock.Unlock()
	sessionsbyuserlock.Lock()
	delete(sessionsbyuser, loginsession.user)
	sessionsbyuserlock.Unlock()
	return true
}

func usertags(token []byte) []string {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return nil
	}
	
	return loginsession.tags	
}

func userchangepassword(passwordConn *mgo.Collection, token []byte, oldpw []byte, newpw []byte) string {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return ERROR_INVALID_TOKEN
	}
	
	var hash []byte
	var err error
	
	user := loginsession.user
	_, err = checkpassword(passwordConn, user, oldpw)
	if err != nil {
		return "Bad password"
	}
	
	hash, err = bcrypt.GenerateFromPassword(newpw, bcrypt.DefaultCost)
	if err != nil {
		return "Server error"
	}
	
	updatepasssel := bson.M{ "user": user }
	updatepasscom := bson.M{ "$set": bson.M{ "password": bson.Binary{ Kind: 0x80, Data: hash } } }
	
	err = passwordConn.Update(updatepasssel, updatepasscom)
	if err == nil {
		return "Success"
	} else {
		return "Server error"
	}
}

/* Periodically purges sessions. */
func purgeSessionsPeriodically(maxAge int64, periodSeconds int64) {
	var period = time.Duration(periodSeconds) * time.Second
	for {
		time.Sleep(period)
		purgeSessions(maxAge)
	}
}

/* Removes sessions that are older than MAXAGE seconds. */
func purgeSessions(maxAge int64) {
	sessionsbyidlock.Lock()
	sessionsbyuserlock.Lock()
	defer sessionsbyidlock.Unlock()
	defer sessionsbyuserlock.Unlock()
	
	var now int64 = time.Now().Unix()
	
	for _, session := range sessionsbyid {
		if now - session.lastUsed > maxAge {
			delete(sessionsbyid, session.token)
			delete(sessionsbyuser, session.user)
		}
	}
}
