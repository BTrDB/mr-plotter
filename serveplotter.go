package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	
	cparse "github.com/SoftwareDefinedBuildings/sync2_quasar/configparser"
	cpint "github.com/SoftwareDefinedBuildings/btrdb/cpinterface"
	capnp "github.com/glycerine/go-capnproto"
	ws "github.com/gorilla/websocket"
	uuid "github.com/pborman/uuid"
)

const (
	QUASAR_LOW int64 = 1 - (16 << 56)
	QUASAR_HIGH int64 = (48 << 56) - 1
	INVALID_TIME int64 = -0x8000000000000000
)

var upgrader = ws.Upgrader{}

func splitTime(time int64) (millis int64, nanos int32) {
	millis = time / 1000000
	nanos = int32(time % 1000000)
	if nanos < 0 {
		nanos += 1000000
		millis++
	}
	return
}

type QueryMessagePart struct {
	segment *capnp.Segment
	request *cpint.Request
	query *cpint.CmdQueryStatisticalValues
}

var queryPool sync.Pool = sync.Pool{
	New: func () interface{} {
		var seg *capnp.Segment = capnp.NewBuffer(nil)
		var req cpint.Request = cpint.NewRootRequest(seg)
		var query cpint.CmdQueryStatisticalValues = cpint.NewCmdQueryStatisticalValues(seg)
		query.SetVersion(0)
		return QueryMessagePart{
			segment: seg,
			request: &req,
			query: &query,
		}
	},
}

type BracketMessagePart struct {
	segment *capnp.Segment
	request *cpint.Request
	bquery *cpint.CmdQueryNearestValue
}

var bracketPool sync.Pool = sync.Pool{
	New: func () interface{} {
		var seg *capnp.Segment = capnp.NewBuffer(nil)
		var req cpint.Request = cpint.NewRootRequest(seg)
		var bquery cpint.CmdQueryNearestValue = cpint.NewCmdQueryNearestValue(seg)
		bquery.SetVersion(0)
		return BracketMessagePart{
			segment: seg,
			request: &req,
			bquery: &bquery,
		}
	},
}

type Writable interface {
	GetWriter () io.Writer
}

type RespWrapper struct {
	wr http.ResponseWriter
}

func (rw RespWrapper) GetWriter() io.Writer {
	return rw.wr
}

type ConnWrapper struct {
	Writing *sync.Mutex
	Conn *ws.Conn
	CurrWriter io.WriteCloser
}

func (cw *ConnWrapper) GetWriter() io.Writer {
	cw.Writing.Lock()
	w, err := cw.Conn.NextWriter(ws.TextMessage)
	if err == nil {
		cw.CurrWriter = w
		return w
	} else {
		fmt.Printf("Could not get writer on WebSocket: %v", err)
		return nil
	}
}

/** DataRequester encapsulates a series of connections used for obtaining data
	from QUASAR. */
type DataRequester struct {
	connections []net.Conn
	sendLocks []*sync.Mutex
	currID uint64
	connID uint32
	pending uint32
	maxPending uint32
	pendingLock *sync.Mutex
	responseWriters map[uint64]Writable
	synchronizers map[uint64]chan bool
	gotFirstSeg map[uint64]bool
	boundaries map[uint64]int64
	alive bool
}

/** Creates a new DataRequester object.
	dbAddr - the address of the database from where to obtain data.
	numConnections - the number of connections to use.
	maxPending - a limit on the maximum number of pending requests.
	bracket - whether or not the new DataRequester will be used for bracket calls. */
func NewDataRequester(dbAddr string, numConnections int, maxPending uint32, bracket bool) *DataRequester {
	var connections []net.Conn = make([]net.Conn, numConnections)
	var locks []*sync.Mutex = make([]*sync.Mutex, numConnections)
	var err error
	var i int
	for i = 0; i < numConnections; i++ {
		connections[i], err = net.Dial("tcp", dbAddr)
		if err != nil {
			fmt.Printf("Could not connect to database at %v: %v\n", dbAddr, err)
			return nil
		}
		locks[i] = &sync.Mutex{}
	}
	
	var dr *DataRequester = &DataRequester{
		connections: connections,
		sendLocks: locks,
		currID: 0,
		connID: 0,
		pending: 0,
		maxPending: maxPending,
		pendingLock: &sync.Mutex{},
		responseWriters: make(map[uint64]Writable),
		synchronizers: make(map[uint64]chan bool),
		gotFirstSeg: make(map[uint64]bool),
		boundaries: make(map[uint64]int64),
		alive: true,
	}
	
	var responseHandler func(net.Conn)
	if bracket {
		responseHandler = dr.handleBracketResponse
	} else {
		responseHandler = dr.handleDataResponse
	}
	
	for i = 0; i < numConnections; i++ {
		go responseHandler(connections[i])
	}
	
	return dr
}

/* Makes a request for data and writes the result to the specified Writer. */
func (dr *DataRequester) MakeDataRequest(uuidBytes uuid.UUID, startTime int64, endTime int64, pw uint8, writ Writable) {
	for true {
		dr.pendingLock.Lock()
		if dr.pending < dr.maxPending {
			dr.pending += 1
			dr.pendingLock.Unlock()
			break
		} else {
			dr.pendingLock.Unlock()
			time.Sleep(time.Second)
		}
	}
	
	defer atomic.AddUint32(&dr.pending, 0xFFFFFFFF)
	
	var mp QueryMessagePart = queryPool.Get().(QueryMessagePart)
	
	segment := mp.segment
	request := mp.request
	query := mp.query
	
	query.SetUuid([]byte(uuidBytes))
	query.SetStartTime(startTime)
	query.SetEndTime(endTime)
	query.SetPointWidth(pw)
	
	id := atomic.AddUint64(&dr.currID, 1)
	
	request.SetEchoTag(id)
	
	request.SetQueryStatisticalValues(*query)
	
	cid := atomic.AddUint32(&dr.connID, 1) % uint32(len(dr.connections))
	
	dr.sendLocks[cid].Lock()
	dr.responseWriters[id] = writ
	dr.synchronizers[id] = make(chan bool)
	dr.gotFirstSeg[id] = false
	_, sendErr := segment.WriteTo(dr.connections[cid])
	dr.sendLocks[cid].Unlock()
	
	defer delete(dr.responseWriters, id)
	defer delete(dr.synchronizers, id)
	defer delete(dr.gotFirstSeg, id)
	
	queryPool.Put(mp)
	
	if sendErr != nil {
		w := writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Could not send query to database: %v", sendErr)))
		return
	}
	
	<- dr.synchronizers[id]
}

/** A function designed to handle QUASAR's response over Cap'n Proto.
	You shouldn't ever have to invoke this function. It is used internally by
	the constructor function. */
func (dr *DataRequester) handleDataResponse(connection net.Conn) {
	for dr.alive {
		// Only one goroutine will be reading at a time, so a lock isn't needed
		responseSegment, respErr := capnp.ReadFromStream(connection, nil)
		
		if respErr != nil {
			if !dr.alive {
				break
			}
			fmt.Printf("Error in receiving response: %v\n", respErr)
			continue
		}
		
		responseSeg := cpint.ReadRootResponse(responseSegment)
		id := responseSeg.EchoTag()
		status := responseSeg.StatusCode()
		records := responseSeg.StatisticalRecords().Values()
		final := responseSeg.Final()
		
		firstSeg := !dr.gotFirstSeg[id]
		writ := dr.responseWriters[id]
		
		dr.gotFirstSeg[id] = true
		
		w := writ.GetWriter()
		
		if status != cpint.STATUSCODE_OK {
			fmt.Printf("Bad status code: %v\n", status)
			w.Write([]byte(fmt.Sprintf("Database returns status code %v", status)))
			if final {
				dr.synchronizers[id] <- false
			}
			continue
		}
		
		length := records.Len()
		
		if firstSeg {
			w.Write([]byte("["))
		}
		for i := 0; i < length; i++ {
			record := records.At(i)
			millis, nanos := splitTime(record.Time())
			if firstSeg && i == 0 {
				w.Write([]byte(fmt.Sprintf("[%v,%v,%v,%v,%v,%v]", millis, nanos, record.Min(), record.Mean(), record.Max(), record.Count())))
			} else {
				w.Write([]byte(fmt.Sprintf(",[%v,%v,%v,%v,%v,%v]", millis, nanos, record.Min(), record.Mean(), record.Max(), record.Count())))
			}
		}
		
		if final {
			w.Write([]byte("]"))
		}
		
		if final {
			dr.synchronizers[id] <- true
		}
	}
}

func (dr *DataRequester) MakeBracketRequest(uuids []uuid.UUID, writ Writable) {
	for true {
		dr.pendingLock.Lock()
		if dr.pending < dr.maxPending {
			dr.pending += 1
			dr.pendingLock.Unlock()
			break
		} else {
			dr.pendingLock.Unlock()
			time.Sleep(time.Second)
		}
	}
	
	defer atomic.AddUint32(&dr.pending, 0xFFFFFFFF)
	
	var mp BracketMessagePart = bracketPool.Get().(BracketMessagePart)
	
	segment := mp.segment
	request := mp.request
	bquery := mp.bquery
	
	var numResponses int = 2 * len(uuids)
	var responseChan chan bool = make(chan bool, numResponses)
	
	var idsUsed []uint64 = make([]uint64, numResponses) // Due to concurrency, we could use a non-contiguous block of IDs
	
	var i int
	var id uint64
	var cid uint32
	var sendErr error
	for i = 0; i < len(uuids); i++ {
		bquery.SetUuid([]byte(uuids[i]))
		bquery.SetTime(QUASAR_LOW)
		bquery.SetBackward(false)
	
		id = atomic.AddUint64(&dr.currID, 1)
		idsUsed[i << 1] = id
		dr.boundaries[id] = INVALID_TIME
	
		request.SetEchoTag(id)
	
		request.SetQueryNearestValue(*bquery)
	
		cid = atomic.AddUint32(&dr.connID, 1) % uint32(len(dr.connections))
	
		dr.sendLocks[cid].Lock()
		dr.responseWriters[id] = writ
		dr.synchronizers[id] = responseChan
		_, sendErr = segment.WriteTo(dr.connections[cid])
		dr.sendLocks[cid].Unlock()
		
		defer delete(dr.responseWriters, id)
		defer delete(dr.synchronizers, id)
		defer delete(dr.boundaries, id)
		
		if sendErr != nil {
			w := writ.GetWriter()
			w.Write([]byte(fmt.Sprintf("Could not send query to database: %v", sendErr)))
			return
		}
		
		bquery.SetTime(QUASAR_HIGH)
		bquery.SetBackward(true)
		
		id = atomic.AddUint64(&dr.currID, 1)
		idsUsed[(i << 1) + 1] = id
		dr.boundaries[id] = INVALID_TIME
	
		request.SetEchoTag(id)
	
		request.SetQueryNearestValue(*bquery)
	
		cid = atomic.AddUint32(&dr.connID, 1) % uint32(len(dr.connections))
	
		dr.sendLocks[cid].Lock()
		dr.responseWriters[id] = writ
		dr.synchronizers[id] = responseChan
		_, sendErr = segment.WriteTo(dr.connections[cid])
		dr.sendLocks[cid].Unlock()
		
		defer delete(dr.responseWriters, id)
		defer delete(dr.synchronizers, id)
		defer delete(dr.boundaries, id)
		
		if sendErr != nil {
			w := writ.GetWriter()
			w.Write([]byte(fmt.Sprintf("Could not send query to database: %v", sendErr)))
			return
		}
	}
	
	bracketPool.Put(mp)
	
	for i = 0; i < numResponses; i++ {
		<- responseChan
	}
	
	var (
		boundary int64
		lNanos int32
		lMillis int64
		rNanos int32
		rMillis int64
		lowest int64 = QUASAR_HIGH
		highest int64 = QUASAR_LOW
		trailchar rune = ','
	)
	w := writ.GetWriter()
	w.Write([]byte("{\"Brackets\": ["))
	for i = 0; i < len(uuids); i++ {
		boundary = dr.boundaries[idsUsed[i << 1]]
		if boundary < lowest {
			lowest = boundary
		}
		lMillis, lNanos = splitTime(boundary)
		boundary = dr.boundaries[idsUsed[(i << 1) + 1]]
		if boundary > highest {
			highest = boundary
		}
		rMillis, rNanos = splitTime(boundary)
		if i == len(uuids) - 1 {
			trailchar = ']';
		}
		w.Write([]byte(fmt.Sprintf("[[%v,%v],[%v,%v]]%c", lMillis, lNanos, rMillis, rNanos, trailchar)))
	}
	lMillis, lNanos = splitTime(lowest)
	rMillis, rNanos = splitTime(highest)
	w.Write([]byte(fmt.Sprintf(",\"Merged\":[[%v,%v],[%v,%v]]}", lMillis, lNanos, rMillis, rNanos)))
}

/** A function designed to handle QUASAR's response over Cap'n Proto.
	You shouldn't ever have to invoke this function. It is used internally by
	the constructor function. */
func (dr *DataRequester) handleBracketResponse(connection net.Conn) {
	for dr.alive {
		// Only one goroutine will be reading at a time, so a lock isn't needed
		responseSegment, respErr := capnp.ReadFromStream(connection, nil)
		
		if respErr != nil {
			if !dr.alive {
				break
			}
			fmt.Printf("Error in receiving response: %v\n", respErr)
			continue
		}
		
		responseSeg := cpint.ReadRootResponse(responseSegment)
		id := responseSeg.EchoTag()
		status := responseSeg.StatusCode()
		records := responseSeg.Records().Values()
		
		if status != cpint.STATUSCODE_OK {
			fmt.Printf("Error in bracket call: database returns status code %v\n", status)
			dr.synchronizers[id] <- false
			continue
		}
		
		if records.Len() > 0 {
			dr.boundaries[id] = records.At(0).Time()
		}
		
		dr.synchronizers[id] <- true
	}
}

func (dr *DataRequester) stop() {
	dr.alive = false
}

func parseDataRequest(request string, writ Writable) (uuidBytes uuid.UUID, startTime int64, endTime int64, pw uint8, extra string, success bool) {
	var args []string = strings.Split(string(request), ",")
	var err error
	
	success = false
	var w io.Writer

	if len(args) != 4 && len(args) != 5 {
		w = writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Four or five arguments are required; got %v", len(args))))
		return
	}
	
	if len(args) == 5 {
		extra = args[4]
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

func parseBracketRequest(request string, writ Writable, expectExtra bool) (uuids []uuid.UUID, extra string, success bool) {
	var args []string = strings.Split(string(request), ",")
	
	success = false
	var w io.Writer

	var numUUIDs int
	
	if expectExtra {
		numUUIDs = len(args) - 1
		if numUUIDs < 1 {
			w = writ.GetWriter()
			w.Write([]byte(fmt.Sprintf("At least two arguments are required; got %v", len(args))))
			return
		}
		extra = args[numUUIDs]
	} else {
		numUUIDs = len(args)
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
	
	success = true
	
	return
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
		
		uuidBytes, startTime, endTime, pw, echoTag, success := parseDataRequest(string(payload), &cw)
		fmt.Println("Got data request")
	
		if success {
			dr.MakeDataRequest(uuidBytes, startTime, endTime, uint8(pw), &cw)
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

	// TODO: don't just read the whole thing in one go. Instead give up after a reasonably long limit.
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Could not read received POST payload: %v", err)))
	}
	
	wrapper := RespWrapper{w}
	
	uuidBytes, startTime, endTime, pw, _, success := parseDataRequest(string(payload), wrapper)
	
	if success {
		dr.MakeDataRequest(uuidBytes, startTime, endTime, uint8(pw), wrapper)
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
		
		uuids, echoTag, success := parseBracketRequest(string(payload), &cw, true)
		
		if success {
			br.MakeBracketRequest(uuids, &cw)
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
	
	uuids, _, success := parseBracketRequest(string(payload), wrapper, false)
	
	if success {
		br.MakeBracketRequest(uuids, wrapper)
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

func logoffHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("To log off, make a POST request with the session token."))
		return
	}
	
	tokenencoded := make([]byte, token64len, token64len)
	tokenslice := make([]byte, token64dlen, token64dlen)
	
	n, err := io.ReadFull(r.Body, tokenencoded)
	if err == nil && n == token64len {
		n, err = base64.StdEncoding.Decode(tokenslice, tokenencoded)
		if n == TOKEN_BYTE_LEN && err == nil && userlogoff(tokenslice) {
			w.Write([]byte("Logoff successful."))
			return
		}
	}
	
	w.Write([]byte("Invalid session token."))
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
