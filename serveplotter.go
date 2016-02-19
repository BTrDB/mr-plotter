package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	
	cparse "github.com/SoftwareDefinedBuildings/sync2_quasar/configparser"
	uuid "github.com/pborman/uuid"
	ws "github.com/gorilla/websocket"
)

var upgrader = ws.Upgrader{}

type RespWrapper struct {
	wr io.Writer
}

func (rw RespWrapper) GetWriter() io.Writer {
	return rw.wr
}

/* State needed to handle HTTP requests. */
var dr *DataRequester
var br *DataRequester
var mdServer string
var permalinkConn *mgo.Collection
var accountConn *mgo.Collection
var csvURL string
var token64len int
var token64dlen int

func main() {
	configfile, err := ioutil.ReadFile("plotter.ini")
	if err != nil {
		fmt.Printf("Could not read plotter.ini: %v\n", err)
		return
	}
	
	config, isErr := cparse.ParseConfig(string(configfile))
	if isErr {
		fmt.Println("There were errors while parsing plotter.ini. See above.")
		return
	}
	
	port, ok := config["port"]
	if !ok {
		fmt.Println("Configuration file is missing required key \"port\"")
		return
	}
	
	dbaddr, ok := config["db_addr"]
	if !ok {
		fmt.Println("Configuration file is missing required key \"db_addr\"")
		return
	}
	
	dataConnRaw, ok := config["num_data_conn"]
	if !ok {
		fmt.Println("Configuration file is missing required key \"num_data_conn\"")
		return
	}
	
	bracketConnRaw, ok := config["num_bracket_conn"]
	if !ok {
		fmt.Println("Configuration file is missing required key \"num_data_conn\"")
		return
	}
	
	directory, ok := config["plotter_dir"]
	if !ok {
		fmt.Println("Configuration file is missing required key \"plotter_dir\"")
		return
	}
	
	mdServerRaw, ok := config["metadata_server"]
	if !ok {
		fmt.Println("Configuration file is missing required key \"metadata_server\"")
		return
	}
	
	mgServerRaw, ok := config["mongo_server"]
	if !ok {
		fmt.Println("Configuration file is missing required key \"mongo_server\"")
		return
	}
	
	csvURLRaw, ok := config["csv_url"]
	if !ok {
		fmt.Println("Configuration file is missing required key \"csv_url\"")
		return
	}
	
	dataConn64, err := strconv.ParseInt(dataConnRaw.(string), 0, 64)
	if err != nil {
		fmt.Println("Configuration file must specify num_data_conn as an int")
		return
	}
	bracketConn64, err := strconv.ParseInt(bracketConnRaw.(string), 0, 64)
	if err != nil {
		fmt.Println("Configuration file must specify num_bracket_conn as an int")
		return
	}
	var dataConn int = int(dataConn64)
	var bracketConn int = int(bracketConn64)
	mdServer = mdServerRaw.(string)
	mgServer := mgServerRaw.(string)
	csvURL = csvURLRaw.(string)
	
	mongoConn, err := mgo.Dial(mgServer)
	if err != nil {
		fmt.Printf("Could not connect to MongoDB Server at address %s\n", mgServer)
		os.Exit(1)
	}
	
	plotterDBConn := mongoConn.DB("mr_plotter")
	permalinkConn = plotterDBConn.C("permalinks")
	accountConn = plotterDBConn.C("accounts")
	
	dr = NewDataRequester(dbaddr.(string), dataConn, 8, false)
	if dr == nil {
		os.Exit(1)
	}
	br = NewDataRequester(dbaddr.(string), bracketConn, 8, true)
	if br == nil {
		os.Exit(1)
	}
	
	token64len = base64.StdEncoding.EncodedLen(TOKEN_BYTE_LEN)
	token64dlen = base64.StdEncoding.DecodedLen(token64len)
	
	http.Handle("/", http.FileServer(http.Dir(directory.(string))))
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
	
	var portStr string = fmt.Sprintf(":%v", port)
	
	certFile, ok1 := config["cert_file"]
	keyFile, ok2 := config["key_file"]
	if ok1 && ok2 {
		log.Fatal(http.ListenAndServeTLS(portStr, certFile.(string), keyFile.(string), nil))
	} else {
		fmt.Println("Not using TLS: cert_file and key_file not specified in plotter.ini")
		log.Fatal(http.ListenAndServe(portStr, nil))
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
	if err != nil || len(tokenslice) != TOKEN_BYTE_LEN {
		return nil
	}
	return getloginsession(tokenslice)
}

func datawsHandler(w http.ResponseWriter, r *http.Request) {
	websocket, upgradeerr := upgrader.Upgrade(w, r, nil)
	if upgradeerr != nil {
		// TODO Perhaps we could redirect somehow?
		w.Write([]byte(fmt.Sprintf("Could not upgrade HTTP connection to WebSocket: %v\n", upgradeerr)))
		return
	}
	
	cw := ConnWrapper{
		Writing: &sync.Mutex{},
		Conn: websocket,
	}
	
	for {
		_, payload, err := websocket.ReadMessage()
		
		if err != nil {
			return // Most likely the connection was closed
		}
		
		uuidBytes, startTime, endTime, pw, token, echoTag, success := parseDataRequest(string(payload), &cw)
	
		if success {
			var loginsession *LoginSession
			if len(token) != 0 {
				loginsession = validateToken(token)
				if loginsession == nil {
					w.Write([]byte("invalid token"))
					return
				}
			}
			if hasPermission(loginsession, uuidBytes) {
				dr.MakeDataRequest(uuidBytes, startTime, endTime, uint8(pw), &cw)
			} else {
				cw.GetWriter().Write([]byte("[]"))
			}
		}
		if cw.CurrWriter != nil {
			cw.CurrWriter.Close()
		}
		
		writer, err := websocket.NextWriter(ws.TextMessage)
		if err != nil {
			fmt.Println("Could not echo tag to client")
		}
		
		if cw.CurrWriter != nil {
			_, err = writer.Write([]byte(echoTag))
			if err != nil {
				fmt.Println("Could not echo tag to client")
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
	
	var gzipWriter *gzip.Writer

	// TODO: don't just read the whole thing in one go. Instead give up after a reasonably long limit.
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Could not read received POST payload: %v", err)))
		return
	}
	
	var wrapper RespWrapper = RespWrapper{w}
	
	uuidBytes, startTime, endTime, pw, token, _, success := parseDataRequest(string(payload), wrapper)
		
	if success {
		var loginsession *LoginSession
		if len(token) != 0 {
			loginsession = validateToken(token)
			if loginsession == nil {
				w.Write([]byte("invalid token"))
				return
			}
		}
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			gzipWriter = gzip.NewWriter(w)
			defer gzipWriter.Close()
		
			wrapper = RespWrapper{gzipWriter}
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", "application/json")
		}
		if hasPermission(loginsession, uuidBytes) {
			dr.MakeDataRequest(uuidBytes, startTime, endTime, uint8(pw), wrapper)
		} else {
			wrapper.GetWriter().Write([]byte("[]"))
		}
	}
}

func bracketwsHandler(w http.ResponseWriter, r *http.Request) {
	websocket, upgradeerr := upgrader.Upgrade(w, r, nil)
	if upgradeerr != nil {
		// TODO Perhaps we could redirect somehow?
		w.Write([]byte(fmt.Sprintf("Could not upgrade HTTP connection to WebSocket: %v\n", upgradeerr)))
		return
	}
	
	cw := ConnWrapper{
		Writing: &sync.Mutex{},
		Conn: websocket,
	}
	
	for {
		_, payload, err := websocket.ReadMessage()
		
		if err != nil {
			return // Most likely the connection was closed
		}
		
		uuids, token, echoTag, success := parseBracketRequest(string(payload), &cw, true)
		
		if success {
			var loginsession *LoginSession
			if len(token) != 0 {
				loginsession = validateToken(token)
				if loginsession == nil {
					w.Write([]byte("invalid token"))
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
			if canview {
				br.MakeBracketRequest(uuids, &cw)
			} else {
				br.MakeBracketRequest([]uuid.UUID{}, &cw)
			}
		}
		if cw.CurrWriter != nil {
			cw.CurrWriter.Close()
		}
		
		writer, err := websocket.NextWriter(ws.TextMessage)
		if err != nil {
			fmt.Println("Could not echo tag to client")
		}
		
		if cw.CurrWriter != nil {
			_, err = writer.Write([]byte(echoTag))
			if err != nil {
				fmt.Println("Could not echo tag to client")
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

	// TODO: don't just read the whole thing in one go. Instead give up after a reasonably long limit.
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Could not read received POST payload: %v", err)))
	}
	
	wrapper := RespWrapper{w}
	
	uuids, token, _, success := parseBracketRequest(string(payload), wrapper, false)
	
	if success {
		var loginsession *LoginSession
		if len(token) != 0 {
			loginsession = validateToken(token)
			if loginsession == nil {
				w.Write([]byte("invalid token"))
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
		if canview {
			br.MakeBracketRequest(uuids, wrapper)
		} else {
			br.MakeBracketRequest([]uuid.UUID{}, wrapper)
		}
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
		
		if len(tokenencoded) == token64len {
			tokenslice := make([]byte, token64dlen, token64dlen)
			n, err = base64.StdEncoding.Decode(tokenslice, tokenencoded)
			if n == TOKEN_BYTE_LEN && err == nil {
				tagslice := usertags(tokenslice)
				if tagslice != nil {
					tags = strings.Join(tagslice, ",")
				}
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
	
	var buffer []byte = make([]byte, 1024) // forward the response in 1 KiB chunks
	
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
	
	if r.Method == "GET" {
		r.ParseForm()
		var id64str string = r.Form.Get("id")
		if id64str == "" {
			w.Write([]byte(PERMALINK_HELP))
			return
		}
		
		var idslice []byte
		idslice, err = base64.URLEncoding.DecodeString(id64str)
		
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
			fmt.Printf("Could not update permalink record: %v\n", err)
		}
		
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		
		var permalinkEncoder *json.Encoder = json.NewEncoder(w)
		err = permalinkEncoder.Encode(jsonPermalink)
		
		if err != nil {
			fmt.Printf("Could not encode permlink data: %v\n", err)
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
	w.Header().Set("Content-Disposition", "attachment; filename=data.csv")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	
	csvReq, err := http.NewRequest("POST", csvURL, r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Could not perform HTTP request to database: %v", err)))
		return
	}
	
	csvReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(csvReq)
	
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprintf("Could not forward request to database: %v", err)))
		return
	}
	
	var buffer []byte = make([]byte, 4096) // forward the response in 4 KiB chunks
	
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
		token64buf := make([]byte, token64len)
		base64.StdEncoding.Encode(token64buf, tokenarr)
		w.Write(token64buf)
	}
}

func parseToken(reader io.Reader) []byte {
	tokenencoded := make([]byte, token64len, token64len)
	tokenslice := make([]byte, token64dlen, token64dlen)
	
	n, err := io.ReadFull(reader, tokenencoded)
	if err == nil && n == token64len {
		n, err = base64.StdEncoding.Decode(tokenslice, tokenencoded)
		if n == TOKEN_BYTE_LEN && err == nil {
			return tokenslice
		}
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
	
	tokenslice := parseToken(r.Body)
	
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
	
	if len(token) != token64len {
		w.Write([]byte("Error: invalid token"))
		return
	}
	
	tokenslice, err = base64.StdEncoding.DecodeString(token)
	if err != nil || len(tokenslice) != TOKEN_BYTE_LEN {
		w.Write([]byte("Error: invalid token"))
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
	
	tokenslice := parseToken(r.Body)
	
	if tokenslice != nil && getloginsession(tokenslice) != nil {
		w.Write([]byte("ok"))
	} else {
		w.Write([]byte("invalid"))
	}
}
