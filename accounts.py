#!/usr/bin/python
import bcrypt
import bson.binary
import pymongo

client = pymongo.MongoClient()
accounts = client.mr_plotter.accounts

def disp_help(tokens):
    print "Type one of the following commands and press <Enter> or <Return> to execute it:"
    print " ".join(funcs.keys())

def create_funcs():
    help = disp_help
    
    def adduser(tokens):
        if len(tokens) < 3:
            print "Usage: adduser username password [tag1] [tag2] ..."
            return
        username = tokens[1]
        password = tokens[2]
        phash = bcrypt.hashpw(password, bcrypt.gensalt())
        pbin = bson.binary.Binary(phash, bson.binary.USER_DEFINED_SUBTYPE)
        # ALL accounts have the public tag
        tags = set(tokens[3:])
        tags.add("public")
        accounts.update({"user": username}, {"$set": {"password": pbin, "tags": tuple(tags)}}, upsert = True)
        
    def setpassword(tokens):
        if len(tokens) != 3:
            print "Usage: setpassword username password"
            return
        username = tokens[1]
        password = tokens[2]
        phash = bcrypt.hashpw(password, bcrypt.gensalt())
        pbin = bson.binary.Binary(phash, bson.binary.USER_DEFINED_SUBTYPE)
        accounts.update({"user": username}, {"$set": {"password": pbin}})
            
    def rmuser(tokens):
        if len(tokens) != 2:
            print "Usage: rmuser username"
            return
        accounts.delete_one({"user": tokens[1]})
            
    def addtags(tokens):
        if len(tokens) < 3:
            print "Usage: addtags username tag1 [tag2] [tag3] ..."
            return
        accounts.update({"user": tokens[1]}, {"$addToSet": {"tags": {"$each": tokens[2:]}}})
            
    def rmtags(tokens):
        if len(tokens) < 3:
            print "Usage: rmtags username tag1 [tag2] [tag3] ..."
            return
        # ALL accounts have the public tag
        tags = set(tokens[2:])
        if "public" in tags:
            tags.remove("public")
        accounts.update({"user": tokens[1]}, {"$pullAll": {"tags": tuple(tags)}})
            
    def lstags(tokens):
        if len(tokens) != 2:
            print "Usage: lstags username"
            return
        account = accounts.find_one({"user": tokens[1]})
        print " ".join(account["tags"])
            
    def lsusers(tokens):
        if len(tokens) != 1:
            print "Usage: lsusers"
            return
        for account in accounts.find():
            print account["user"]
            
    def ls(tokens):
        if len(tokens) != 1:
            print "Usage: ls"
            return
        for account in accounts.find():
            print "{0}: {1}".format(account["user"], " ".join(account["tags"]))
        
    def exit(tokens):
        if len(tokens) != 1:
            print "Usage: exit"
            return
        raise SystemExit
        
    def close(tokens):
        if len(tokens) != 1:
            print "Usage: close"
            return
        raise SystemExit
        
    return locals()
    
funcs = create_funcs()

def exec_statement(string):
    tokens = string.split()
    if len(tokens) == 0:
        return
    
    command = tokens[0]
    if command not in funcs:
        exec_func = disp_help
        print "'{0}' is not a valid command".format(command)
    else:
        exec_func = funcs[command]
    
    exec_func(tokens)
    
while True:
    try:
        statement = raw_input("Mr. Plotter> ")
    except EOFError:
        print
        raise SystemExit
    exec_statement(statement)
