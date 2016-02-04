from BaseHTTPServer import BaseHTTPRequestHandler, HTTPServer
from SocketServer import ThreadingMixIn
import json
import os
import pymongo
import re
import requests
import string
import sys
import urllib

def doc_matches_path(stream_doc, pathstarts):
    for start in pathstarts:
        if stream_doc['Path'].startswith(start):
            return True
    return False

client = pymongo.MongoClient()
mongo_collection = client.qdf.metadata
try:
    configfile = open(sys.argv[-1], 'r')
    data = configfile.read()
    configfile.close()
except BaseException as be:
    print be
    print 'You must specify a file name as an argument. The file must be a JSON document that maps each tag to a list of path-start strings'
    exit()
    
tag_defs = json.loads(data)
class HTTPRequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        self.wfile.write('GET request received')
        
    def do_POST(self):
        tags = None
        if self.path.find('?') != -1:
            arg_string = self.path.split('?')[1]
            arg_pairs = map(lambda x: x.split('='), arg_string.split('&'))
            args = {pair[0]: pair[1] for pair in arg_pairs}
            if 'tags' in args:
                tag_string = args['tags']
                tags = tag_string.split(',')
        if not tags:
            tags = ['public'] # if no tags are given, assume this
        pathstarts = set()
        for tag in tags:
            if tag in tag_defs:
                pathstarts.update(tag_defs[tag])
        
        self.query = self.rfile.read(int(self.headers['Content-Length']))
        if len(self.query) == 0:
            self.send_response(400)
            self.end_headers()
            self.wfile.write("Request was empty")
            return
        if self.query[-1] == ';':
            self.query = self.query[:-1]
        self.send_response(200)
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET POST')
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        if self.query == 'select distinct Metadata/SourceName':
            sources = set()
            for stream in mongo_collection.find():
                if not pathstarts or doc_matches_path(stream, pathstarts):
                    sources.add(stream['Metadata']['SourceName'])
            self.wfile.write(json.dumps(list(sources)))
        elif self.query.startswith('select distinct Path where Metadata/SourceName'):
            source = self.query.split('"')[1]
            paths = set()
            for stream in mongo_collection.find({"$where": 'this.Metadata.SourceName === "{0}"'.format(source)}):
                if not pathstarts or doc_matches_path(stream, pathstarts):
                    paths.add(stream['Path'])
            self.wfile.write(json.dumps(list(paths)))
        elif self.query.startswith('select * where Metadata/SourceName'):
            parts = self.query.split('"')
            source = parts[1]
            path = parts[3]
            streams = set()
            for stream in mongo_collection.find({"$where": 'this.Metadata.SourceName === "{0}" && this.Path === "{1}"'.format(source, path)}):
                if not pathstarts or doc_matches_path(stream, pathstarts):
                    del stream['_id']
                    streams.add(json.dumps(stream))
            returnstr = '['
            for stream in streams:
                returnstr += stream + ", "
            self.wfile.write(returnstr[:-2] + ']')
        elif self.query.startswith('select * where uuid ='): # I assume that it's a sequence of ORs
            parts = self.query.split('"')
            uuids = []
            i = 0
            while i < len(parts):
                if i % 2 != 0:
                    uuids.append('this.uuid === "{0}"'.format(parts[i]))
                i += 1
            streams = set()
            for stream in mongo_collection.find({"$where": ' || '.join(uuids)}):
                if not pathstarts or doc_matches_path(stream, pathstarts):
	            del stream['_id']
        	    streams.add(json.dumps(stream))
            returnstr = '['
            for stream in streams:
                returnstr += stream + ", "
            self.wfile.write(returnstr[:-2] + ']')
        else:
            self.wfile.write('[]')
                    
class ThreadedHTTPServer(ThreadingMixIn, HTTPServer):
    pass
        
serv = ThreadedHTTPServer(('', 4523), HTTPRequestHandler)
serv.serve_forever()
