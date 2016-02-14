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
	loginsession = sessionsbyuser[user]
	if loginsession == nil {
		// Need to create a new session
		if MAX_TOKEN.Sign() == 0 {
			(&MAX_TOKEN).SetBytes(MAX_TOKEN_BYTES)
		}
		var token *big.Int
		var tokenarr [TOKEN_BYTE_LEN]byte
		for token == nil || sessionsbyid[tokenarr] != nil {
			token, err = rand.Int(rand.Reader, &MAX_TOKEN)
			if err != nil {
				fmt.Println("Could not generate session key")
				return nil
			}
			tokenbytes := token.Bytes()
		
			copy(tokenarr[TOKEN_BYTE_LEN - len(tokenbytes):], tokenbytes)
		}
		
		loginsession = &LoginSession{
			user: user,
			token: tokenarr,
			tags: taglist,
		}
		
		sessionsbyid[tokenarr] = loginsession
		sessionsbyuser[user] = loginsession
		
		return tokenarr[:]
	} else {
		return loginsession.token[:]
	}
}

func getloginsession(token []byte) *LoginSession {
	var tokenarr [TOKEN_BYTE_LEN]byte
	copy(tokenarr[:], token)
	
	return sessionsbyid[tokenarr]
}

func userlogoff(token []byte) bool {
	loginsession := getloginsession(token)
	if loginsession == nil {
		return false
	}
	
	delete(sessionsbyid, loginsession.token)
	delete(sessionsbyuser, loginsession.user)
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
		return "Bad token"
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
