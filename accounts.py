#!/usr/bin/python
import pymongo

client = pymongo.MongoClient()
accounts_collection = client.mr_plotter.accounts

def disp_help(tokens):
    print "Valid commands are:", " ".join(funcs.keys())

def create_funcs():
    help = disp_help
    
    def adduser(tokens):
        if len(tokens) < 3:
            print "Usage: adduser username password [tag1] [tag2] ..."
            return
        print "Not implemented"
            
    def deluser(tokens):
        if len(tokens) != 2:
            print "Usage: deluser username"
            return
        print "Not implemented"
            
    def addtags(tokens):
        if len(tokens) < 3:
            print "Usage: addtags username tag1 [tag2] [tag3] ..."
            return
        print "Not implemented"
            
    def rmtags(tokens):
        if len(tokens) < 3:
            print "Usage: rmtags username tag1 [tag2] [tag3] ..."
            return
        print "Not implemented"
            
    def lstags(tokens):
        if len(tokens) != 2:
            print "Usage: lstags username"
            return
        print "Not implemented"
            
    def lsusers(tokens):
        if len(tokens) != 1:
            print "Usage: lsusers"
            return
        print "Not implemented"
            
    def lsall(tokens):
        if len(tokens) != 1:
            print "Usage: lsall"
            return
        print "Not implemented"
        
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
        
    exec_func = funcs.get(tokens[0], disp_help)
    exec_func(tokens)
    
while True:
    try:
        statement = raw_input("Mr. Plotter> ")
    except EOFError:
        print
        raise SystemExit
    exec_statement(statement)
