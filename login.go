/* This file contains the logic for users to login and change their password.
   It also contains the logic to keep track of login sessions. */

package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	
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
	user string
	token [TOKEN_BYTE_LEN]byte
	tags []string
}

/* Initialized to the above byte value on use. */
var MAX_TOKEN big.Int = big.Int{}

// Maps ID to session
var sessionsbyid map[[TOKEN_BYTE_LEN]byte]*LoginSession = make(map[[TOKEN_BYTE_LEN]byte]*LoginSession)
// Maps user to session
var sessionsbyuser map[string]*LoginSession = make(map[string]*LoginSession)

/* Writing to the returned slice results in undefined behavior.
   The returned slice is guaranteed to have a length of TOKEN_BYTE_LEN. */
func userlogin(passwordConn *mgo.Collection, user string, password []byte) []byte {
	var userdoc map[string]interface{}
	var loginsession *LoginSession
	var err error
	userquery := bson.M{ "user": user }
	err = passwordConn.Find(userquery).One(&userdoc)
	if err == nil {
		return nil
	}
	hashedpasswordbinary := userdoc["password"].(bson.Binary)
	hashedpassword := hashedpasswordbinary.Data
	
	err = bcrypt.CompareHashAndPassword(hashedpassword, password)
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
	loginsession = sessionsbyuser[user]
	if loginsession == nil {
		// Need to create a new session
		if MAX_TOKEN.Sign() == 0 {
			(&MAX_TOKEN).SetBytes(MAX_TOKEN_BYTES)
		}
		token, err := rand.Int(rand.Reader, &MAX_TOKEN)
		if err != nil {
			fmt.Println("Could not generate session key")
			return nil
		}
		tokenbytes := token.Bytes()
		
		var tokenarr [TOKEN_BYTE_LEN]byte
		copy(tokenarr[TOKEN_BYTE_LEN - len(tokenbytes):], tokenbytes)
		
		loginsession = &LoginSession{
			user: user,
			token: tokenarr,
			tags: taglist,
		}
		return tokenarr[:]
	} else {
		return loginsession.token[:]
	}
}

func getloginsession(token []byte) *LoginSession {
	if len(token) != TOKEN_BYTE_LEN {
		panic(fmt.Sprintf("Logoff token has length %d; expected length %d\n", len(token), TOKEN_BYTE_LEN))
	}
	var tokenarr [TOKEN_BYTE_LEN]byte
	copy(tokenarr[:], token)
	
	return sessionsbyid[tokenarr]
}

func userlogoff(token []byte) {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return
	}
	
	delete(sessionsbyid, loginsession.token)
	delete(sessionsbyuser, loginsession.user)
}

func usertags(token []byte) []string {
	if len(token) != TOKEN_BYTE_LEN {
		return nil
	}
	
	loginsession := getloginsession(token)
	if loginsession == nil {
		return nil
	}
	
	return loginsession.tags	
}
