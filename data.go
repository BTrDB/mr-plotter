/*
 * Copyright (C) 2016 Sam Kumar, Michael Andersen, and the University
 * of California, Berkeley.
 *
 * This file is part of Mr. Plotter (the Multi-Resolution Plotter).
 *
 * Mr. Plotter is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Mr. Plotter is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with Mr. Plotter.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
	
	cpint "github.com/SoftwareDefinedBuildings/btrdb/cpinterface"
	capnp "github.com/glycerine/go-capnproto"
	uuid "github.com/pborman/uuid"
	ws "github.com/gorilla/websocket"
)

const (
	QUASAR_LOW int64 = 1 - (16 << 56)
	QUASAR_HIGH int64 = (48 << 56) - 1
	INVALID_TIME int64 = -0x8000000000000000
)

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
	timeout time.Duration
	currID uint64
	connID uint32
	totalWaiting uint64
	pending uint32
	maxPending uint32
	pendingLock *sync.Mutex
	pendingCondVar *sync.Cond
	synchronizers map[uint64]chan cpint.Response
	stateLock *sync.RWMutex
	alive bool
}

/** Creates a new DataRequester object.
	dbAddr - the address of the database from where to obtain data.
	numConnections - the number of connections to use.
	maxPending - a limit on the maximum number of pending requests.
	timeout - timeout on requests to the database.
	bracket - whether or not the new DataRequester will be used for bracket calls. */
func NewDataRequester(dbAddr string, numConnections int, maxPending uint32, timeout time.Duration, bracket bool) *DataRequester {
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
	
	pendingLock := &sync.Mutex{}
	var dr *DataRequester = &DataRequester{
		connections: connections,
		sendLocks: locks,
		timeout: timeout,
		currID: 0,
		connID: 0,
		totalWaiting: 0,
		pending: 0,
		maxPending: maxPending,
		pendingLock: pendingLock,
		pendingCondVar: sync.NewCond(pendingLock),
		synchronizers: make(map[uint64]chan cpint.Response),
		stateLock: &sync.RWMutex{},
		alive: true,
	}
	
	for i = 0; i < numConnections; i++ {
		go dr.handleResponses(connections[i])
	}
	
	return dr
}

/* Makes a request for data and writes the result to the specified Writer. */
func (dr *DataRequester) MakeDataRequest(uuidBytes uuid.UUID, startTime int64, endTime int64, pw uint8, writ Writable) {
	atomic.AddUint64(&dr.totalWaiting, 1)
	defer atomic.AddUint64(&dr.totalWaiting, 0xFFFFFFFFFFFFFFFF)
	
	dr.pendingLock.Lock()
	for dr.pending == dr.maxPending {
		dr.pendingCondVar.Wait()
	}
	dr.pending += 1
	dr.pendingLock.Unlock()
	
	var respchan = make(chan cpint.Response, 10)
	
	var firstSeg bool
	
	var mp QueryMessagePart = queryPool.Get().(QueryMessagePart)
	
	var timeout *time.Timer
	
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
	
	dr.stateLock.Lock()
	dr.synchronizers[id] = respchan
	dr.stateLock.Unlock()
	
	fmt.Printf("Issuing data request %v (UUID = %s, start = %v, end = %v, pw = %v)\n", id, uuidBytes.String(), startTime, endTime, pw)
	dr.sendLocks[cid].Lock()
	_, sendErr := segment.WriteTo(dr.connections[cid])
	dr.sendLocks[cid].Unlock()
	
	queryPool.Put(mp)
	
	w := writ.GetWriter()
	if sendErr != nil {
		fmt.Printf("Data request %v FAILS: %v\n", id, sendErr)
		
		w.Write([]byte(fmt.Sprintf("Could not send query to database: %v", sendErr)))
		goto finish
	}
	
	firstSeg = true
	timeout = time.NewTimer(dr.timeout)
	
readloop:
	for {
		select {
		case responseSeg := <-respchan:
			status := responseSeg.StatusCode()
			records := responseSeg.StatisticalRecords().Values()
			final := responseSeg.Final()
		
			fmt.Printf("Got data response for request %v: FirstSeg = %v, FinalSeg = %v\n", id, firstSeg, final)
		
			if status != cpint.STATUSCODE_OK {
				fmt.Printf("Bad status code: %v\n", status)
				w.Write([]byte(fmt.Sprintf("Database returns status code %v", status)))
				break readloop
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
				break readloop
			}
		
			firstSeg = false
			
		case <-timeout.C:
			fmt.Printf("WARNING: request %v (UUID = %s, start = %v, end = %v, pw = %v) timed out\n", id, uuidBytes.String(), startTime, endTime, pw)
			w.Write([]byte("Timed out"))
			goto finish
		}
	}
	
	/* If I reached this case, it means that we finished processing the data and the timeout didn't expire.
	   So we need to free the timer resource. */
	if !timeout.Stop() {
		// If timeout.Stop() returned false, it means that the timeout fired during the last iteration.
		<-timeout.C
	}
	
finish:
	dr.stateLock.Lock()
	delete(dr.synchronizers, id)
	dr.stateLock.Unlock()
	
	dr.pendingLock.Lock()
	if dr.pending == dr.maxPending {
		dr.pending -= 1
		dr.pendingCondVar.Signal()
	} else {
		dr.pending -= 1
	}
	dr.pendingLock.Unlock()
}

func (dr *DataRequester) MakeBracketRequest(uuids []uuid.UUID, writ Writable) {
	atomic.AddUint64(&dr.totalWaiting, 1)
	defer atomic.AddUint64(&dr.totalWaiting, 0xFFFFFFFFFFFFFFFF)
	
	dr.pendingLock.Lock()
	for dr.pending == dr.maxPending {
		dr.pendingCondVar.Wait()
	}
	dr.pending += 1
	dr.pendingLock.Unlock()
	
	var mp BracketMessagePart = bracketPool.Get().(BracketMessagePart)
	
	segment := mp.segment
	request := mp.request
	bquery := mp.bquery
	
	var numResponses uint64 = uint64(len(uuids)) << 1
	var responseChan chan cpint.Response = make(chan cpint.Response, numResponses)
	
	/* Get a contiguous block of IDs. */
	var startNext uint64 = atomic.AddUint64(&dr.currID, numResponses) + 1
	var startID uint64 = startNext - numResponses
	
	var i int
	var j uint64
	var id uint64 = startID
	var cid uint32
	var sendErr error
	
	var timeout *time.Timer
	
	var boundarySlice []int64 = make([]int64, numResponses)
	
	// For final processing once all responses are received
	var (
		boundary int64
		lNanos int32
		lMillis int64
		rNanos int32
		rMillis int64
		lowest int64 = QUASAR_HIGH
		highest int64 = QUASAR_LOW
		trailchar rune = ','
		w io.Writer
	)
	
	dr.stateLock.Lock()
	for id = startID; id != startNext; id++ {
		dr.synchronizers[id] = responseChan
	}
	dr.stateLock.Unlock()
	
	id = startID
	
	for i = 0; i < len(uuids); i++ {
		bquery.SetUuid([]byte(uuids[i]))
		bquery.SetTime(QUASAR_LOW)
		bquery.SetBackward(false)
	
		request.SetEchoTag(id)
	
		request.SetQueryNearestValue(*bquery)
	
		cid = atomic.AddUint32(&dr.connID, 1) % uint32(len(dr.connections))
		
		fmt.Printf("Issuing bracket request %v\n", id)
		dr.sendLocks[cid].Lock()
		_, sendErr = segment.WriteTo(dr.connections[cid])
		dr.sendLocks[cid].Unlock()
		
		if sendErr != nil {
			w := writ.GetWriter()
			w.Write([]byte(fmt.Sprintf("Could not send query to database: %v", sendErr)))
			bracketPool.Put(mp)
			goto finish
		}
		
		id += 1
		
		bquery.SetTime(QUASAR_HIGH)
		bquery.SetBackward(true)
	
		request.SetEchoTag(id)
	
		request.SetQueryNearestValue(*bquery)
	
		cid = atomic.AddUint32(&dr.connID, 1) % uint32(len(dr.connections))
		
		fmt.Printf("Issuing bracket request %v\n", id)
		dr.sendLocks[cid].Lock()
		_, sendErr = segment.WriteTo(dr.connections[cid])
		dr.sendLocks[cid].Unlock()
		
		if sendErr != nil {
			w := writ.GetWriter()
			w.Write([]byte(fmt.Sprintf("Could not send query to database: %v", sendErr)))
			bracketPool.Put(mp)
			goto finish
		}
		
		id += 1
	}
	
	bracketPool.Put(mp)
	
	timeout = time.NewTimer(dr.timeout)
	w = writ.GetWriter()
	for j = 0; j < numResponses; j++ {
		select {
		case responseSeg := <-responseChan:
			id := responseSeg.EchoTag()
			status := responseSeg.StatusCode()
			records := responseSeg.Records().Values()
		
			fmt.Printf("Got bracket response for request %v\n", id)
		
			if status != cpint.STATUSCODE_OK || records.Len() == 0 {
				fmt.Printf("Error in bracket call request %v: database returns status code %v\n", id, status)
				boundarySlice[id - startID] = INVALID_TIME
				continue
			} else {
				boundarySlice[id - startID] = records.At(0).Time()
			}
		case <-timeout.C:
			fmt.Printf("WARNING: bracket request in [%v, %v) timed out\n", startID, startNext)
			w.Write([]byte("Timed out"))
			goto exit
		}
	}
	
	/* If I reached this case, it means that we finished processing the data and the timeout didn't expire.
	   So we need to free the timer resource. */
	if !timeout.Stop() {
		// If timeout.Stop() returned false, it means that the timeout fired during the last iteration.
		<-timeout.C
	}
	
finish:
	dr.stateLock.Lock()
	for id = startID; id != startNext; id++ {
		delete(dr.synchronizers, id)
	}
	dr.stateLock.Unlock()
	w.Write([]byte("{\"Brackets\": ["))
	
	for i = 0; i < len(uuids); i++ {
		boundary = boundarySlice[i << 1]
		if boundary != INVALID_TIME && boundary < lowest {
			lowest = boundary
		}
		lMillis, lNanos = splitTime(boundary)
		boundary = boundarySlice[(i << 1) + 1]
		if boundary != INVALID_TIME && boundary > highest {
			highest = boundary
		}
		rMillis, rNanos = splitTime(boundary)
		if i == len(uuids) - 1 {
			trailchar = ']';
		}
		w.Write([]byte(fmt.Sprintf("[[%v,%v],[%v,%v]]%c", lMillis, lNanos, rMillis, rNanos, trailchar)))
	}
	if len(uuids) == 0 {
		w.Write([]byte("]"))
	}
	lMillis, lNanos = splitTime(lowest)
	rMillis, rNanos = splitTime(highest)
	w.Write([]byte(fmt.Sprintf(",\"Merged\":[[%v,%v],[%v,%v]]}", lMillis, lNanos, rMillis, rNanos)))
	
exit:
	dr.pendingLock.Lock()
	if dr.pending == dr.maxPending {
		dr.pending -= 1
		dr.pendingCondVar.Signal()
	} else {
		dr.pending -= 1
	}
	dr.pendingLock.Unlock()
}


/** A function designed to handle QUASAR's response over Cap'n Proto.
	You shouldn't ever have to invoke this function. It is used internally by
	the constructor function. */
func (dr *DataRequester) handleResponses(connection net.Conn) {
	for dr.alive {
		// Only one goroutine will be reading at a time, so a lock isn't needed
		responseSegment, respErr := capnp.ReadFromStream(connection, nil)
		
		if respErr != nil {
			if !dr.alive {
				break
			}
			fmt.Printf("Error in receiving response: %v\n", respErr)
			os.Exit(1)
		}
		
		responseSeg := cpint.ReadRootResponse(responseSegment)
		id := responseSeg.EchoTag()
		
		fmt.Printf("Got response to request %v\n", id)
		
		dr.stateLock.RLock()
		respchan := dr.synchronizers[id]
		dr.stateLock.RUnlock()
		
		if respchan == nil {
			fmt.Printf("Dropping extraneous response for request %v\n", id)
		} else {
			respchan <- responseSeg
		}
	}
}

func (dr *DataRequester) stop() {
	dr.alive = false
}
