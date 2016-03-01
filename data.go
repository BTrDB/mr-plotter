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
	"sync"
	"sync/atomic"
	
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
	currID uint64
	connID uint32
	totalWaiting uint64
	pending uint32
	maxPending uint32
	pendingLock *sync.Mutex
	pendingCondVar *sync.Cond
	responseWriters map[uint64]Writable
	synchronizers map[uint64]chan bool
	stateLock *sync.RWMutex
	gotFirstSeg map[uint64]bool
	gotFirstSegLock *sync.RWMutex
	boundaries map[uint64]int64
	boundaryLock *sync.Mutex
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
	
	pendingLock := &sync.Mutex{}
	var dr *DataRequester = &DataRequester{
		connections: connections,
		sendLocks: locks,
		currID: 0,
		connID: 0,
		totalWaiting: 0,
		pending: 0,
		maxPending: maxPending,
		pendingLock: pendingLock,
		pendingCondVar: sync.NewCond(pendingLock),
		responseWriters: make(map[uint64]Writable),
		synchronizers: make(map[uint64]chan bool),
		stateLock: &sync.RWMutex{},
		gotFirstSeg: make(map[uint64]bool),
		gotFirstSegLock: &sync.RWMutex{},
		boundaries: make(map[uint64]int64),
		boundaryLock: &sync.Mutex{},
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
	atomic.AddUint64(&dr.totalWaiting, 1)
	defer atomic.AddUint64(&dr.totalWaiting, 0xFFFFFFFFFFFFFFFF)
	
	dr.pendingLock.Lock()
	for dr.pending == dr.maxPending {
		dr.pendingCondVar.Wait()
	}
	dr.pending += 1
	dr.pendingLock.Unlock()
	
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
	
	dr.stateLock.Lock()
	dr.responseWriters[id] = writ
	syncchan := make(chan bool)
	dr.synchronizers[id] = syncchan
	dr.stateLock.Unlock()
	
	dr.gotFirstSegLock.Lock()
	dr.gotFirstSeg[id] = false
	dr.gotFirstSegLock.Unlock()
	
	fmt.Printf("Issuing data request %v\n", id)
	dr.sendLocks[cid].Lock()
	_, sendErr := segment.WriteTo(dr.connections[cid])
	dr.sendLocks[cid].Unlock()
	
	queryPool.Put(mp)
	
	if sendErr != nil {
		fmt.Printf("Data request %v FAILS: %v\n", id, sendErr)
		
		w := writ.GetWriter()
		w.Write([]byte(fmt.Sprintf("Could not send query to database: %v", sendErr)))
		goto finish
	}
	
	<- syncchan
	
	finish:
	dr.stateLock.Lock()
	delete(dr.responseWriters, id)
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
		
		dr.gotFirstSegLock.RLock()
		_, firstSeg := dr.gotFirstSeg[id]
		dr.gotFirstSegLock.RUnlock()
		if firstSeg {
			dr.gotFirstSegLock.Lock()
			delete(dr.gotFirstSeg, id)
			dr.gotFirstSegLock.Unlock()
		}
		
		dr.stateLock.RLock()
		writ := dr.responseWriters[id]
		syncchan := dr.synchronizers[id]
		dr.stateLock.RUnlock()
		
		fmt.Printf("Got data response for request %v: FirstSeg = %v, FinalSeg = %v\n", id, firstSeg, final)
		
		w := writ.GetWriter()
		
		if status != cpint.STATUSCODE_OK {
			fmt.Printf("Bad status code: %v\n", status)
			w.Write([]byte(fmt.Sprintf("Database returns status code %v", status)))
			if final {
				syncchan <- false
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
			syncchan <- true
		}
	}
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
	
	var numResponses int = len(uuids) << 1
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
	
		request.SetEchoTag(id)
	
		request.SetQueryNearestValue(*bquery)
	
		cid = atomic.AddUint32(&dr.connID, 1) % uint32(len(dr.connections))
		
		dr.stateLock.Lock()
		dr.synchronizers[id] = responseChan
		dr.stateLock.Unlock()
		
		fmt.Printf("Issuing bracket request %v\n", id)
		dr.sendLocks[cid].Lock()
		_, sendErr = segment.WriteTo(dr.connections[cid])
		dr.sendLocks[cid].Unlock()
		
		if sendErr != nil {
			w := writ.GetWriter()
			w.Write([]byte(fmt.Sprintf("Could not send query to database: %v", sendErr)))
			return
		}
		
		bquery.SetTime(QUASAR_HIGH)
		bquery.SetBackward(true)
		
		id = atomic.AddUint64(&dr.currID, 1)
		idsUsed[(i << 1) + 1] = id
	
		request.SetEchoTag(id)
	
		request.SetQueryNearestValue(*bquery)
	
		cid = atomic.AddUint32(&dr.connID, 1) % uint32(len(dr.connections))
	
		dr.stateLock.Lock()
		dr.synchronizers[id] = responseChan
		dr.stateLock.Unlock()
		
		fmt.Printf("Issuing bracket request %v\n", id)
		dr.sendLocks[cid].Lock()
		_, sendErr = segment.WriteTo(dr.connections[cid])
		dr.sendLocks[cid].Unlock()
		
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
		ok bool
		boundarySlice []int64 = make([]int64, numResponses)
		lNanos int32
		lMillis int64
		rNanos int32
		rMillis int64
		lowest int64 = QUASAR_HIGH
		highest int64 = QUASAR_LOW
		trailchar rune = ','
	)
	dr.boundaryLock.Lock()
	for i = 0; i < numResponses; i++ {
		boundarySlice[i], ok = dr.boundaries[idsUsed[i]]
		if !ok {
			boundarySlice[i] = INVALID_TIME
		}
		delete(dr.boundaries, idsUsed[i])
	}
	dr.boundaryLock.Unlock()
	
	dr.stateLock.Lock()
	for i = 0; i < numResponses; i++ {
		delete(dr.synchronizers, idsUsed[i])
	}
	dr.stateLock.Unlock()
	
	w := writ.GetWriter()
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
		
		dr.stateLock.RLock()
		syncchan := dr.synchronizers[id]
		dr.stateLock.RUnlock()
		
		fmt.Printf("Got bracket response for request %v\n", id)
		
		if status != cpint.STATUSCODE_OK {
			fmt.Printf("Error in bracket call: database returns status code %v\n", status)
			syncchan <- false
			continue
		}
		
		if records.Len() > 0 {
			dr.boundaryLock.Lock()
			dr.boundaries[id] = records.At(0).Time()
			dr.boundaryLock.Unlock()
		}
		
		syncchan <- true
	}
}

func (dr *DataRequester) stop() {
	dr.alive = false
}
