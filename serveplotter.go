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

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/btrdb.v4"

	"gopkg.in/ini.v1"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	httpHandlers "github.com/gorilla/handlers"
	uuid "github.com/pborman/uuid"
	ws "github.com/gorilla/websocket"
)

const (
	FORWARD_CHUNKSIZE int = (4 << 10) // 4 KiB
	MAX_REQSIZE int64 = (16 << 10) // 16 KiB
	ERROR_INVALID_TOKEN string = "Invalid token"

	MONGO_ID_LEN int = 12
)

type CSVRequest struct {
	StartTime int64
	EndTime int64
	UUIDs []string `json:"UUIDS"`
	Labels []string
	UnitofTime string
	Token string `json:"_token,omitempty"`
	PointWidth uint8
}

var upgrader = ws.Upgrader{}

type RespWrapper struct {
	wr io.Writer
}

func (rw RespWrapper) GetWriter() io.Writer {
	return rw.wr
}

/* State needed to handle HTTP requests. */
var btrdbConn *btrdb.BTrDB
var dr *DataRequester
var br *DataRequester
var mdServer string
var permalinkConn *mgo.Collection
var accountConn *mgo.Collection
var csvURL string
var permalinklen int
var permalinkdlen int
var csvMaxPoints int64
var dataTimeout time.Duration
var bracketTimeout time.Duration

/* I don't order these elements from largest to smallest, so the int64s at the
   bottom may not be 8-byte aligned. That's OK, because I don't anticipate
   doing any atomic operations on these, and regular operations don't have to
   be particularly fast (I'm just parsing a config file, after all). */
type Config struct {
	HttpPort uint16
	HttpsPort uint16
	UseHttp bool
	UseHttps bool
	HttpsRedirect bool
	LogHttpRequests bool
	CompressHttpResponses bool
	PlotterDir string
	CertFile string
	KeyFile string

	SessionEncryptKeyFile string
	SessionMacKeyFile string

	BtrdbEndpoints []string
	NumDataConn uint16
	NumBracketConn uint16
	MaxDataRequests uint32
	MaxBracketRequests uint32
	MaxCachedTagPermissions uint64
	MetadataServer string
	MongoServer string
	CsvUrl string

	SessionExpirySeconds uint64
	SessionPurgeIntervalSeconds int64
	CsvMaxPointsPerStream int64
	OutstandingRequestLogInterval int64
	NumGoroutinesLogInterval int64
	DbDataTimeoutSeconds int64
	DbBracketTimeoutSeconds int64
}

var configRequiredKeys = map[string]bool{
	"http_port": true,
	"https_port": true,
	"use_http": true,
	"use_https": true,
	"https_redirect": true,
	"log_http_requests": true,
	"compress_http_responses": true,
	"plotter_dir": true,
	"cert_file": true,
	"key_file": true,

	"session_encrypt_key_file": true,
	"session_mac_key_file": true,

	"btrdb_endpoints": true,
	"max_data_requests": true,
	"max_bracket_requests": true,
	"max_cached_tag_permissions": true,
	"metadata_server": true,
	"mongo_server": true,
	"csv_url": true,

	"session_expiry_seconds": true,
	"session_purge_interval_seconds": true,
	"csv_max_points_per_stream": true,
	"outstanding_request_log_interval": true,
	"num_goroutines_log_interval": true,
	"db_data_timeout_seconds": true,
	"db_bracket_timeout_seconds": true,
}

func main() {
	var config Config
	var filename string

	if len(os.Args) < 2 {
		filename = "plotter.ini"
	} else {
		filename = os.Args[1]
	}

	rawConfig, err := ini.Load(filename)
	if err != nil {
		log.Fatalf("Could not parse %s: %v", filename, err)
	}

	/* Validate the configuration file. */
	defaultSect := rawConfig.Section("")
	for requiredKey, _ := range configRequiredKeys {
		if !defaultSect.HasKey(requiredKey) {
			log.Fatalf("Configuration file is missing required key \"%s\"", requiredKey)
		}
	}

	rawConfig.NameMapper = ini.TitleUnderscore
	err = rawConfig.MapTo(&config)
	if err != nil {
		log.Fatalf("Could not map configuration file: %v", err)
	}

	ekey, err := ioutil.ReadFile(config.SessionEncryptKeyFile)
	if err != nil {
		log.Fatalf("Could not read encryption key file: %v", err)
	}
	mkey, err := ioutil.ReadFile(config.SessionMacKeyFile)
	if err != nil {
		log.Fatalf("Could not read MAC key file: %v", err)
	}
	if bytes.Equal(ekey, mkey) {
		log.Fatalln("The session encryption and MAC keys are the same; to ensure that session state is stored securely on the client, please change them to be different")
	}

	err = setEncryptKey(ekey)
	if err != nil {
		log.Fatalf("Invalid encryption key: %v", err)
	}
	err = setMACKey(mkey)
	if err != nil {
		log.Fatalf("Invalid MAC key: %v", err)
	}

	setTagPermissionCacheSize(config.MaxCachedTagPermissions)

	mdServer = config.MetadataServer
	csvURL = config.CsvUrl
	csvMaxPoints = config.CsvMaxPointsPerStream

	log.Printf("Connecting to MongoDB... ")
	mongoConn, err := mgo.Dial(config.MongoServer)
	if err != nil {
		log.Fatalf("Error: %v\n", err)
	}
	log.Println("Successfully connected to MongoDB")

	plotterDBConn := mongoConn.DB("mr_plotter")
	permalinkConn = plotterDBConn.C("permalinks")
	accountConn = plotterDBConn.C("accounts")

	log.Printf("Connecting to BTrDB cluster...")
	btrdbConn, err = btrdb.Connect(context.Background(), config.BtrdbEndpoints...)
	if err != nil {
		log.Fatalf("Error: %v\n", err)
		os.Exit(1)
	}
	log.Println("Successfully connected to BTrDB")

	dataTimeout = time.Duration(config.DbDataTimeoutSeconds) * time.Second
	bracketTimeout = time.Duration(config.DbBracketTimeoutSeconds) * time.Second

	dr = NewDataRequester(btrdbConn, config.MaxDataRequests)
	if dr == nil {
		os.Exit(1)
	}
	br = NewDataRequester(btrdbConn, config.MaxBracketRequests)
	if br == nil {
		os.Exit(1)
	}

	setSessionExpiry(config.SessionExpirySeconds)

	go logWaitingRequests(time.Duration(config.OutstandingRequestLogInterval) * time.Second)
	go logNumGoroutines(time.Duration(config.NumGoroutinesLogInterval) * time.Second)

	permalinklen = base64.URLEncoding.EncodedLen(MONGO_ID_LEN)
	permalinkdlen = base64.URLEncoding.DecodedLen(permalinklen)

	http.Handle("/", http.FileServer(http.Dir(config.PlotterDir)))
	http.HandleFunc("/dataws", datawsHandler)
	http.HandleFunc("/data", dataHandler)
	http.HandleFunc("/bracketws", bracketwsHandler)
	http.HandleFunc("/bracket", bracketHandler)
	http.HandleFunc("/metadata", metadataHandler)
	http.HandleFunc("/permalink", permalinkHandler)
	http.HandleFunc("/csv", csvHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logoff", logoffHandler)
	http.HandleFunc("/changepw", changepwHandler)
	http.HandleFunc("/checktoken", checktokenHandler)

	var mrPlotterHandler http.Handler = http.DefaultServeMux
	if config.LogHttpRequests {
		mrPlotterHandler = httpHandlers.CombinedLoggingHandler(os.Stdout, mrPlotterHandler)
	}
	if config.CompressHttpResponses {
		mrPlotterHandler = httpHandlers.CompressHandler(mrPlotterHandler)
	}

	var portStrHTTP string = fmt.Sprintf(":%d", config.HttpPort)
	var portStrHTTPS string = fmt.Sprintf(":%d", config.HttpsPort)
	if config.UseHttp && config.UseHttps {
		go func () {
				log.Fatal(http.ListenAndServeTLS(portStrHTTPS, config.CertFile, config.KeyFile, mrPlotterHandler))
				os.Exit(1)
			}()

		if config.HttpsRedirect {
			var redirect http.Handler = http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
					var url *url.URL = r.URL
					url.Scheme = "https"
					url.Host = r.Host + portStrHTTPS
					http.Redirect(w, r, url.String(), http.StatusFound)
				})
			var loggedRedirect http.Handler = httpHandlers.CompressHandler(httpHandlers.CombinedLoggingHandler(os.Stdout, redirect))
			log.Fatal(http.ListenAndServe(portStrHTTP, loggedRedirect))
		} else {
			log.Fatal(http.ListenAndServe(portStrHTTP, mrPlotterHandler))
		}
	} else if config.UseHttps {
		log.Fatal(http.ListenAndServeTLS(portStrHTTPS, config.CertFile, config.KeyFile, mrPlotterHandler))
	} else if config.UseHttp {
		log.Fatal(http.ListenAndServe(portStrHTTP, mrPlotterHandler))
	}
	os.Exit(1);
}

func logWaitingRequests(period time.Duration) {
	for {
		time.Sleep(period)
		log.Printf("Waiting data requests: %v; Waiting bracket requests: %v", dr.totalWaiting, br.totalWaiting)
	}
}

func logNumGoroutines(period time.Duration) {
	for {
		time.Sleep(period)
		log.Printf("Number of goroutines: %v", runtime.NumGoroutine())
	}
}

func parseDataRequest(request string, writ Writable) (uuidBytes uuid.UUID, startTime int64, endTime int64, pw uint8, extra1 string, extra2 string, success bool) {
	var args []string = strings.Split(string(request), ",")
	var err error

	success = false
	var w io.Writer

	if len(args) != 4 && len(args) != 5 && len(args) != 6 {
		w = writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Four, five, or six arguments are required; got %v", len(args))))
		return
	}

	if len(args) == 6 {
		extra1 = args[4]
		extra2 = args[5]
	} else if len(args) == 5 {
		extra1 = args[4]
	}

	uuidBytes = uuid.Parse(args[0])

	if uuidBytes == nil {
		w = writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Invalid UUID: got %v", args[0])))
		return
	}
	var pwTemp int64

	startTime, err = strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		w = writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Could not interpret %v as an int64: %v", args[1], err)))
		return
	}

	endTime, err = strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		w = writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Could not interpret %v as an int64: %v", args[2], err)))
		return
	}

	pwTemp, err = strconv.ParseInt(args[3], 10, 16)
	if err != nil {
		w = writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Could not interpret %v as an int16: %v", args[3], err)))
		return
	}

	pw = uint8(pwTemp)

	startTime = ((startTime >> pw) << pw)
	endTime = (((endTime >> pw) + 1) << pw) // we add one pointwidth to the endtime to simulate an inclusive endpoint

	success = true

	return
}

func parseBracketRequest(request string, writ Writable, expectExtra bool) (uuids []uuid.UUID, token string, extra string, success bool) {
	var args []string = strings.Split(string(request), ",")

	success = false
	var w io.Writer

	var numUUIDs int

	if expectExtra {
		numUUIDs = len(args) - 2
	} else {
		numUUIDs = len(args) - 1
	}

	if numUUIDs < 1 {
		w = writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Got only %v arguments", len(args))))
		return
	}

	if expectExtra {
		extra = args[numUUIDs + 1]
	}

	uuids = make([]uuid.UUID, numUUIDs)

	for i := 0; i < numUUIDs; i++ {
		uuids[i] = uuid.Parse(args[i])
		if uuids[i] == nil {
			w = writ.GetWriter()
			w.Write([]byte(fmt.Sprintf("Received invalid UUID %v", args[i])))
			return
		}
	}

	token = args[numUUIDs]

	success = true

	return
}

func validateToken(token string) *LoginSession {
	tokenslice, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil
	}
	return getloginsession(tokenslice)
}

func datawsHandler(w http.ResponseWriter, r *http.Request) {
	var websocket *ws.Conn
	var upgradeerr error
	websocket, upgradeerr = upgrader.Upgrade(w, r, nil)
	if upgradeerr != nil {
		w.Write([]byte(fmt.Sprintf("Could not upgrade HTTP connection to WebSocket: %v\n", upgradeerr)))
		return
	}

	cw := ConnWrapper{
		Writing: &sync.Mutex{},
		Conn: websocket,
	}

	websocket.SetReadLimit(MAX_REQSIZE)

	for {
		_, payload, err := websocket.ReadMessage()

		if err != nil {
			return // Most likely the connection was closed or the message was too big
		}

		uuidBytes, startTime, endTime, pw, token, echoTag, success := parseDataRequest(string(payload), &cw)

		if success {
			var loginsession *LoginSession
			if token != "" {
				loginsession = validateToken(token)
				if loginsession == nil {
					w.Write([]byte(ERROR_INVALID_TOKEN))
					return
				}
			}
			if hasPermission(loginsession, uuidBytes) {
				var ctx context.Context
				var cancelfunc context.CancelFunc
				ctx, cancelfunc = context.WithTimeout(context.Background(), dataTimeout)
				dr.MakeDataRequest(ctx, uuidBytes, startTime, endTime, uint8(pw), &cw)
				cancelfunc()
			} else {
				cw.GetWriter().Write([]byte("[]"))
			}
		}
		if cw.CurrWriter != nil {
			cw.CurrWriter.Close()
		}

		writer, err := websocket.NextWriter(ws.TextMessage)
		if err != nil {
			log.Printf("Could not echo tag to client: %v", err)
		}

		if cw.CurrWriter != nil {
			_, err = writer.Write([]byte(echoTag))
			if err != nil {
				log.Printf("Could not echo tag to client: %v", err)
			}
			writer.Close()
		}

		cw.Writing.Unlock()
	}
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("You must send a POST request to get data."))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Could not read received POST payload: %v", err)))
		return
	}

	var wrapper RespWrapper = RespWrapper{w}

	uuidBytes, startTime, endTime, pw, token, _, success := parseDataRequest(string(payload), wrapper)

	if success {
		var loginsession *LoginSession
		if token != "" {
			loginsession = validateToken(token)
			if loginsession == nil {
				w.Write([]byte(ERROR_INVALID_TOKEN))
				return
			}
		}
		if hasPermission(loginsession, uuidBytes) {
			var ctx context.Context
			var cancelfunc context.CancelFunc
			ctx, cancelfunc = context.WithTimeout(context.Background(), dataTimeout)
			dr.MakeDataRequest(ctx, uuidBytes, startTime, endTime, uint8(pw), wrapper)
			cancelfunc()
		} else {
			wrapper.GetWriter().Write([]byte("[]"))
		}
	}
}

func bracketwsHandler(w http.ResponseWriter, r *http.Request) {
	var websocket *ws.Conn
	var upgradeerr error
	websocket, upgradeerr = upgrader.Upgrade(w, r, nil)
	if upgradeerr != nil {
		w.Write([]byte(fmt.Sprintf("Could not upgrade HTTP connection to WebSocket: %v\n", upgradeerr)))
		return
	}

	cw := ConnWrapper{
		Writing: &sync.Mutex{},
		Conn: websocket,
	}

	websocket.SetReadLimit(MAX_REQSIZE)

	for {
		_, payload, err := websocket.ReadMessage()

		if err != nil {
			return // Most likely the connection was closed or the message was too big
		}

		uuids, token, echoTag, success := parseBracketRequest(string(payload), &cw, true)

		if success {
			var loginsession *LoginSession
			if token != "" {
				loginsession = validateToken(token)
				if loginsession == nil {
					w.Write([]byte(ERROR_INVALID_TOKEN))
					return
				}
			}
			var viewable []uuid.UUID = uuids[:0]
			for _, uuid := range uuids {
				if hasPermission(loginsession, uuid) {
					viewable = append(viewable, uuid)
				}
			}
			var ctx context.Context
			var cancelfunc context.CancelFunc
			ctx, cancelfunc = context.WithTimeout(context.Background(), bracketTimeout)
			br.MakeBracketRequest(ctx, uuids, &cw)
			cancelfunc()
		}
		if cw.CurrWriter != nil {
			cw.CurrWriter.Close()
		}

		writer, err := websocket.NextWriter(ws.TextMessage)
		if err != nil {
			log.Printf("Could not echo tag to client: %v", err)
		}

		if cw.CurrWriter != nil {
			_, err = writer.Write([]byte(echoTag))
			if err != nil {
				log.Printf("Could not echo tag to client: %v", err)
			}
			writer.Close()
		}

		cw.Writing.Unlock()
	}
}

func bracketHandler (w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("You must send a POST request to get data."))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Could not read received POST payload: %v", err)))
	}

	wrapper := RespWrapper{w}

	uuids, token, _, success := parseBracketRequest(string(payload), wrapper, false)

	if success {
		var loginsession *LoginSession
		if token != "" {
			loginsession = validateToken(token)
			if loginsession == nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(ERROR_INVALID_TOKEN))
				return
			}
		}
		var canview bool = true
		for _, uuid := range uuids {
			if !hasPermission(loginsession, uuid) {
				canview = false
				break
			}
		}
		var ctx context.Context
		var cancelfunc context.CancelFunc
		ctx, cancelfunc = context.WithTimeout(context.Background(), dataTimeout)
		if canview {
			br.MakeBracketRequest(ctx, uuids, wrapper)
		} else {
			br.MakeBracketRequest(ctx, []uuid.UUID{}, wrapper)
		}
		cancelfunc()
	}
}

func metadataHandler (w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("You must send a POST request to get data."))
		return
	}

	var n int

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	request, err := ioutil.ReadAll(r.Body) // should probably limit the size of this
	if err != nil {
		w.Write([]byte("Could not read request."))
		return
	}

	var tags string = "public"
	semicolonindex := bytes.IndexByte(request, ';')
	if semicolonindex != -1 {
		tokenencoded := request[semicolonindex + 1:]
		request = request[:semicolonindex + 1]

		tokenslice := make([]byte, base64.StdEncoding.DecodedLen(len(tokenencoded)))
		n, err = base64.StdEncoding.Decode(tokenslice, tokenencoded)
		if err == nil {
			tokenslice = tokenslice[:n]
			tagslice := usertags(tokenslice)
			if tagslice != nil {
				tags = strings.Join(tagslice, ",")
			}
		}
	}

	mdReq, err := http.NewRequest("POST", fmt.Sprintf("%s?tags=%s", mdServer, tags), strings.NewReader(string(request)))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Could not perform HTTP request to metadata server: %v", err)))
		return
	}

	mdReq.Header.Set("Content-Type", "text")
	mdReq.Header.Set("Content-Length", fmt.Sprintf("%v", len(request)))
	resp, err := http.DefaultClient.Do(mdReq)

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprintf("Could not forward request to metadata server: %v", err)))
		return
	}

	var buffer []byte = make([]byte, FORWARD_CHUNKSIZE) // forward the response

	var bytesRead int
	var readErr error = nil
	for readErr == nil {
		bytesRead, readErr = resp.Body.Read(buffer)
		w.Write(buffer[:bytesRead])
	}
	resp.Body.Close()
}

const PERMALINK_HELP string = "To create a permalink, send the data as a JSON document via a POST request. To retrieve a permalink, set a GET request, specifying \"id=<permalink identifier>\" in the URL."
const PERMALINK_BAD_ID string = "not found"
func permalinkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		w.Header().Set("Allow", "GET POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(PERMALINK_HELP))
		return
	}

	var err error
	var jsonPermalink map[string]interface{}
	var id bson.ObjectId

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	if r.Method == "GET" {
		r.ParseForm()
		var id64str string = r.Form.Get("id")
		if id64str == "" {
			w.Write([]byte(PERMALINK_HELP))
			return
		}

		/* For backwards-compatibility with permalinks from the Meteor plotter, only look at the first 16 bytes. */
		var idslice []byte = make([]byte, permalinkdlen)
		_, err = base64.URLEncoding.Decode(idslice, []byte(id64str)[:permalinklen])

		if err != nil {
			w.Write([]byte(PERMALINK_HELP))
			return
		}

		id = bson.ObjectId(idslice)

		if !id.Valid() {
			w.Write([]byte(PERMALINK_BAD_ID))
			return
		}

		var query *mgo.Query = permalinkConn.FindId(id)

		err = query.One(&jsonPermalink)
		if err != nil {
			w.Write([]byte(PERMALINK_BAD_ID))
			return
		}

		// I could do this asynchronously, but I think this is good enough
		err = permalinkConn.UpdateId(id, map[string]interface{}{
			"$set": map[string]interface{}{
				"lastAccessed": bson.Now(),
			},
		})

		if err != nil {
			// In the future I could try something like restarting the connection
			log.Printf("Could not update permalink record: %v", err)
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		var permalinkEncoder *json.Encoder = json.NewEncoder(w)
		err = permalinkEncoder.Encode(jsonPermalink)

		if err != nil {
			log.Printf("Could not encode permlink data: %v", err)
		}
	} else {
		var permalinkDecoder *json.Decoder = json.NewDecoder(r.Body)

		err = permalinkDecoder.Decode(&jsonPermalink)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("Error: received invalid JSON: %v", err)))
			return
		}

		err = validatePermalinkJSON(jsonPermalink)
		if err != nil {
			w.Write([]byte(err.Error()))
			return
		}

		id = bson.NewObjectId()
		jsonPermalink["_id"] = id
		jsonPermalink["lastAccessed"] = bson.Now()

		err = permalinkConn.Insert(jsonPermalink)

		if err == nil {
			id64len := base64.URLEncoding.EncodedLen(len(id))
			id64buf := make([]byte, id64len, id64len)
			base64.URLEncoding.Encode(id64buf, []byte(id))
			w.Write(id64buf)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("Could not add permalink to database: %v", err)))
		}
	}
}

func csvHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("To get a CSV file, send the required data as a JSON document via a POST request."))
		return
	}

	var err error

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	_, err = io.ReadFull(r.Body, make([]byte, 5)) // Remove the "json="
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad request"))
		return
	}

	var jsonCSVReq CSVRequest
	var jsonCSVReqDecoder *json.Decoder = json.NewDecoder(r.Body)
	err = jsonCSVReqDecoder.Decode(&jsonCSVReq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Malformed request"))
		return
	}

	if jsonCSVReq.PointWidth > 62 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("Invalid point width: %d", jsonCSVReq.PointWidth)))
		return
	}

	/* Check the number of points per stream to see if this request is reasonable. */
	var deltaT int64 = jsonCSVReq.EndTime - jsonCSVReq.StartTime

	/* Taken from the BTrDB HTTP interface bindings, to make sure I handle the units in the same way. */
	switch jsonCSVReq.UnitofTime {
	case "":
		fallthrough
	case "ms":
		deltaT *= 1000000
	case "ns":
	case "us":
		deltaT *= 1000
	case "s":
		deltaT *= 1000000000
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("Invalid unit of time: must be 'ns', 'ms', 'us' or 's' (got '%s')", jsonCSVReq.UnitofTime)))
		return
	}

	var pps int64 = deltaT >> jsonCSVReq.PointWidth
	if deltaT & ((1 << jsonCSVReq.PointWidth) - 1) != 0 {
		pps += 1
	}
	if pps > csvMaxPoints {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("CSV file too big: estimated %d points", pps)))
		return
	}

	var loginsession *LoginSession
	if jsonCSVReq.Token != "" {
		loginsession = validateToken(jsonCSVReq.Token)
		if loginsession == nil {
			w.WriteHeader(http.StatusBadRequest)
			// Don't use ERROR_INVALID_TOKEN since this opens on a new page, not in the plotting application
			w.Write([]byte("Session expired"))
			return
		}
	}

	for _, uuidstr := range jsonCSVReq.UUIDs {
		uuidobj := uuid.Parse(uuidstr)
		if uuidobj == nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Malformed UUID"))
			return
		}
		if !hasPermission(loginsession, uuidobj) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Insufficient permissions"))
			return
		}
	}

	// Don't send the token to BTrDB
	jsonCSVReq.Token = ""

	w.Header().Set("Content-Disposition", "attachment; filename=data.csv")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")

	var csvJSON []byte
	csvJSON, err = json.Marshal(&jsonCSVReq)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Could not forward request: %v", err)))
		return
	}

	var csvReq *http.Request
	csvReq, err = http.NewRequest("POST", csvURL, bytes.NewReader(csvJSON))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Could not perform HTTP request to database: %v", err)))
		return
	}

	csvReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(csvReq)

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprintf("Could not forward request to database: %v", err)))
		resp.Body.Close()
		return
	}

	var buffer []byte = make([]byte, FORWARD_CHUNKSIZE) // forward the response in 4 KiB chunks

	var bytesRead int
	var readErr error = nil
	for readErr == nil {
		bytesRead, readErr = resp.Body.Read(buffer)
		w.Write(buffer[:bytesRead])
	}

	resp.Body.Close()
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("To log in, make a POST request with JSON containing a username and password."))
		return
	}

	var err error
	var jsonLogin map[string]interface{}
	var usernameint interface{}
	var username string
	var passwordint interface{}
	var password string
	var ok bool

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	var loginDecoder *json.Decoder = json.NewDecoder(r.Body)

	err = loginDecoder.Decode(&jsonLogin)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Error: received invalid JSON: %v", err)))
		return
	}

	usernameint, ok = jsonLogin["username"]
	if !ok {
		w.Write([]byte(fmt.Sprintf("Error: JSON must contain field 'username'")))
		return
	}

	passwordint, ok = jsonLogin["password"]
	if !ok {
		w.Write([]byte(fmt.Sprintf("Error: JSON must contain field 'password'")))
		return
	}

	username, ok = usernameint.(string)
	if !ok {
		w.Write([]byte(fmt.Sprintf("Error: field 'username' must be a string")))
		return
	}

	password, ok = passwordint.(string)
	if !ok {
		w.Write([]byte(fmt.Sprintf("Error: field 'password' must be a string")))
		return
	}

	tokenarr := userlogin(accountConn, username, []byte(password))
	if tokenarr != nil {
		token64buf := make([]byte, base64.StdEncoding.EncodedLen(len(tokenarr)))
		base64.StdEncoding.Encode(token64buf, tokenarr)
		w.Write(token64buf)
	}
}

func parseToken(tokenencoded []byte) []byte {
	tokenslice := make([]byte, base64.StdEncoding.DecodedLen(len(tokenencoded)))
	n, err := base64.StdEncoding.Decode(tokenslice, tokenencoded)
	if err == nil {
		return tokenslice[:n]
	}

	return nil
}

func logoffHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("To log off, make a POST request with the session token."))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	tokenencoded, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Could not read received POST payload: %v", err)))
		return
	}
	tokenslice := parseToken(tokenencoded)

	if tokenslice != nil && userlogoff(tokenslice) {
		w.Write([]byte("Logoff successful."))
	} else {
		w.Write([]byte("Invalid session token."))
	}
}

func changepwHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("To change password, make a POST request with the appropriate JSON document."))
		return
	}

	var err error
	var jsonChangePassword map[string]interface{}
	var tokenint interface{}
	var token string
	var oldpasswordint interface{}
	var oldpassword string
	var newpasswordint interface{}
	var newpassword string
	var ok bool
	var tokenslice []byte

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	var pwDecoder *json.Decoder = json.NewDecoder(r.Body)

	err = pwDecoder.Decode(&jsonChangePassword)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Error: received invalid JSON: %v", err)))
		return
	}

	tokenint, ok = jsonChangePassword["token"]
	if !ok {
		w.Write([]byte("Error: JSON must contain field 'token'"))
		return
	}

	oldpasswordint, ok = jsonChangePassword["oldpassword"]
	if !ok {
		w.Write([]byte("Error: JSON must contain field 'oldpassword'"))
		return
	}

	newpasswordint, ok = jsonChangePassword["newpassword"]
	if !ok {
		w.Write([]byte("Error: JSON must contain field 'newpassword'"))
		return
	}

	token, ok = tokenint.(string)
	if !ok {
		w.Write([]byte("Error: field 'token' must be a string"))
		return
	}

	oldpassword, ok = oldpasswordint.(string)
	if !ok {
		w.Write([]byte("Error: field 'oldpassword' must be a string"))
		return
	}

	newpassword, ok = newpasswordint.(string)
	if !ok {
		w.Write([]byte("Error: field 'newpassword' must be a string"))
		return
	}

	tokenslice, err = base64.StdEncoding.DecodeString(token)
	if err != nil {
		w.Write([]byte(ERROR_INVALID_TOKEN))
		return
	}

	success := userchangepassword(accountConn, tokenslice, []byte(oldpassword), []byte(newpassword))
	w.Write([]byte(success))
}

func checktokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("To check a token, make a POST request with the token in the request body."))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQSIZE)
	tokenencoded, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Could not read received POST payload: %v", err)))
		return
	}
	tokenslice := parseToken(tokenencoded)

	if tokenslice != nil && getloginsession(tokenslice) != nil {
		w.Write([]byte("ok"))
	} else {
		w.Write([]byte(ERROR_INVALID_TOKEN))
	}
}
