/*
 * Copyright (C) 2016, 2017 Sam Kumar, Michael Andersen, and the University
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
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"

	"gopkg.in/BTrDB/btrdb.v4"

	ws "github.com/gorilla/websocket"
	uuid "github.com/pborman/uuid"
)

const (
	QUASAR_LOW   int64 = 1 - (16 << 56)
	QUASAR_HIGH  int64 = (48 << 56) - 1
	INVALID_TIME int64 = -0x8000000000000000
)

func splitTime(time int64) (millis int64, nanos int32) {
	millis = time / 1000000
	nanos = int32(time % 1000000)
	if nanos < 0 {
		nanos += 1000000
		millis--
	}
	return
}

type Writable interface {
	GetWriter() io.Writer
}

type ConnWrapper struct {
	Writing    *sync.Mutex
	Conn       *ws.Conn
	CurrWriter io.WriteCloser
}

func (cw *ConnWrapper) GetWriter() io.Writer {
	cw.Writing.Lock()
	w, err := cw.Conn.NextWriter(ws.TextMessage)
	if err == nil {
		cw.CurrWriter = w
		return w
	} else {
		log.Printf("Could not get writer on WebSocket: %v", err)
		return nil
	}
}

// DataRequester encapsulates a series of connections used for obtaining data
// from QUASAR.
type DataRequester struct {
	totalWaiting   uint64
	pending        uint32
	maxPending     uint32
	pendingLock    *sync.Mutex
	pendingCondVar *sync.Cond
	stateLock      *sync.RWMutex
	alive          bool

	btrdb *btrdb.BTrDB
}

// NewDataRequester creates a new DataRequester object.
// btrdbConn - established connection to a BTrDB cluster.
// maxPending - a limit on the maximum number of pending requests. */
func NewDataRequester(btrdbConn *btrdb.BTrDB, maxPending uint32) *DataRequester {
	pendingLock := &sync.Mutex{}
	var dr *DataRequester = &DataRequester{
		totalWaiting:   0,
		pending:        0,
		maxPending:     maxPending,
		pendingLock:    pendingLock,
		pendingCondVar: sync.NewCond(pendingLock),
		btrdb:          btrdbConn,
	}

	return dr
}

/* Makes a request for data and writes the result to the specified Writer. */
func (dr *DataRequester) MakeDataRequest(ctx context.Context, uuidBytes uuid.UUID, startTime int64, endTime int64, pw uint8, writ Writable) {
	atomic.AddUint64(&dr.totalWaiting, 1)
	defer atomic.AddUint64(&dr.totalWaiting, 0xFFFFFFFFFFFFFFFF)

	dr.pendingLock.Lock()
	for dr.pending == dr.maxPending {
		dr.pendingCondVar.Wait()
	}
	dr.pending += 1
	dr.pendingLock.Unlock()

	defer func() {
		dr.pendingLock.Lock()
		dr.pending -= 1
		dr.pendingCondVar.Signal()
		dr.pendingLock.Unlock()
	}()

	var w io.Writer

	var stream = dr.btrdb.StreamFromUUID(uuidBytes)

	var exists bool
	var err error
	exists, err = stream.Exists(ctx)
	if err != nil || !exists {
		w = writ.GetWriter()
		w.Write([]byte("[]"))
		return
	}

	var results chan btrdb.StatPoint
	var errors chan error
	results, _, errors = stream.AlignedWindows(ctx, startTime, endTime, pw, 0)

	w = writ.GetWriter()
	w.Write([]byte("["))

	var firstpt bool = true
	for statpt := range results {
		millis, nanos := splitTime(statpt.Time)
		if firstpt {
			w.Write([]byte(fmt.Sprintf("[%v,%v,%v,%v,%v,%v]", millis, nanos, statpt.Min, statpt.Mean, statpt.Max, statpt.Count)))
			firstpt = false
		} else {
			w.Write([]byte(fmt.Sprintf(",[%v,%v,%v,%v,%v,%v]", millis, nanos, statpt.Min, statpt.Mean, statpt.Max, statpt.Count)))
		}
	}

	var waserror bool = false
	for err = range errors {
		w.Write([]byte("\nError: "))
		w.Write([]byte(err.Error()))
		waserror = true
	}

	if !waserror {
		w.Write([]byte("]"))
	}
}

func (dr *DataRequester) MakeBracketRequest(ctx context.Context, uuids []uuid.UUID, writ Writable) {
	atomic.AddUint64(&dr.totalWaiting, 1)
	defer atomic.AddUint64(&dr.totalWaiting, 0xFFFFFFFFFFFFFFFF)

	dr.pendingLock.Lock()
	for dr.pending == dr.maxPending {
		dr.pendingCondVar.Wait()
	}
	dr.pending += 1
	dr.pendingLock.Unlock()

	defer func() {
		dr.pendingLock.Lock()
		dr.pending -= 1
		dr.pendingCondVar.Signal()
		dr.pendingLock.Unlock()
	}()

	var numResponses int = len(uuids) << 1
	var boundarySlice []int64 = make([]int64, numResponses)

	var wg sync.WaitGroup

	var stream *btrdb.Stream

	var i int
	for i = 0; i != numResponses; i++ {
		var seconditer bool = ((i & 1) != 0)

		if !seconditer {
			stream = dr.btrdb.StreamFromUUID(uuids[i>>1])

			var exists bool
			var err error
			exists, err = stream.Exists(ctx)
			if err != nil || !exists {
				boundarySlice[i] = INVALID_TIME
				i++
				boundarySlice[i] = INVALID_TIME
				continue
			} else {
				wg.Add(2)
			}
		}

		go func(stream *btrdb.Stream, loc *int64, high bool) {
			var rawpoint btrdb.RawPoint
			var e error
			var ref int64
			if high {
				ref = QUASAR_HIGH
			} else {
				ref = QUASAR_LOW
			}
			rawpoint, _, e = stream.Nearest(ctx, ref, 0, high)

			if e == nil {
				*loc = rawpoint.Time
			} else {
				*loc = INVALID_TIME
			}

			wg.Done()
		}(stream, &boundarySlice[i], seconditer)
	}

	wg.Wait()

	// For final processing once all responses are received
	var (
		boundary  int64
		lNanos    int32
		lMillis   int64
		rNanos    int32
		rMillis   int64
		lowest    int64     = QUASAR_HIGH
		highest   int64     = QUASAR_LOW
		trailchar rune      = ','
		w         io.Writer = writ.GetWriter()
	)

	w.Write([]byte("{\"Brackets\": ["))

	for i = 0; i < len(uuids); i++ {
		boundary = boundarySlice[i<<1]
		if boundary != INVALID_TIME && boundary < lowest {
			lowest = boundary
		}
		lMillis, lNanos = splitTime(boundary)
		boundary = boundarySlice[(i<<1)+1]
		if boundary != INVALID_TIME && boundary > highest {
			highest = boundary
		}
		rMillis, rNanos = splitTime(boundary)
		if i == len(uuids)-1 {
			trailchar = ']'
		}
		w.Write([]byte(fmt.Sprintf("[[%v,%v],[%v,%v]]%c", lMillis, lNanos, rMillis, rNanos, trailchar)))
	}
	if len(uuids) == 0 {
		w.Write([]byte("]"))
	}
	lMillis, lNanos = splitTime(lowest)
	rMillis, rNanos = splitTime(highest)
	w.Write([]byte(fmt.Sprintf(",\"Merged\":[[%v,%v],[%v,%v]]}", lMillis, lNanos, rMillis, rNanos)))
}
