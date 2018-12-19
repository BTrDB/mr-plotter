/*
 * Copyright (C) 2017 Sam Kumar, Michael Andersen, and the University
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

package csvquery

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"time"

	btrdb "gopkg.in/BTrDB/btrdb.v5"
)

const (
	// AlignedWindowsQuery specifies that an Aligned Windows query (i.e.
	// Statistical Values query) should be made.
	AlignedWindowsQuery = iota

	// WindowsQuery specifies that a Windows query should be made.
	WindowsQuery

	// RawQuery specifies that a Raw Values query should be made.
	RawQuery
)

// CSVQuery stores the parameters for a CSV query.
type CSVQuery struct {
	// QueryType should be one of AlignedWindowsQuery, WindowsQuery, or
	// RawQuery. It specifies what data should be in the CSV file for each
	// stream.
	QueryType int

	// StartTime is the start time for the query, in nanoseconds.
	StartTime int64

	// EndTime is the end time for the query, in nanoseconds.
	EndTime int64

	// WindowSize specifies the size of the window, in nanoseconds, for a
	// Windows query. It is ignored for Aligned Windows queries and Raw Values
	// queries.
	WindowSize uint64

	// Depth specifies the point width exponent for AlignedWindows queries, and
	// the maximum depth for Windows queries. It is ignored for Raw Values
	// queries.
	Depth uint8

	// Streams is a slice of streams to query.
	Streams []*btrdb.Stream

	// Versions is a slice of version numbers to query for each stream.
	// Defaults to 0 (most recent version) for all streams if nil.
	Versions []uint64

	// Labels contains the name to use for each stream in the output CSV file.
	Labels []string

	// IncludeVersions specifies whether the version number of each stream
	// should be included in the CSV header.
	IncludeVersions bool
}

func setTimeHeaders(row []string) {
	row[0] = "Timestamp (ns)"
	row[1] = "Human-Readable Time (UTC)"
}

// streambuffer is a buffer for an array of pending requests, allowing the user
// to individually manipulate the response channels in a type-independent way.
type streambuffer interface {
	getTime(i int) int64
	isOpen(i int) bool
	readPoint(i int) (bool, error)
	writePoint(i int, row []string)
	writeEmptyPoint(i int, row []string)
	getHeaderRow(labels []string, includeVersions bool) []string
}

type stabufentry struct {
	stac chan btrdb.StatPoint
	verc chan uint64
	errc chan error
	pt   btrdb.StatPoint
	open bool
}

// stabuffer satisfies the streambuffer interface using statistical points
type stabuffer []stabufentry

func (sb stabuffer) getTime(i int) int64 {
	return sb[i].pt.Time
}

func (sb stabuffer) isOpen(i int) bool {
	return sb[i].open
}

func (sb stabuffer) readPoint(i int) (bool, error) {
	sb[i].pt, sb[i].open = <-sb[i].stac
	if !sb[i].open {
		err := <-sb[i].errc
		return sb[i].open, err
	}
	return sb[i].open, nil
}

func (sb stabuffer) writePoint(i int, row []string) {
	offset := 2 + (i << 2)
	row[offset+0] = fmt.Sprintf("%f", sb[i].pt.Min)
	row[offset+1] = fmt.Sprintf("%f", sb[i].pt.Mean)
	row[offset+2] = fmt.Sprintf("%f", sb[i].pt.Max)
	row[offset+3] = fmt.Sprintf("%d", sb[i].pt.Count)
}

func (sb stabuffer) writeEmptyPoint(i int, row []string) {
	offset := 2 + (i << 2)
	row[offset+0] = ""
	row[offset+1] = ""
	row[offset+2] = ""
	row[offset+3] = ""
}

func (sb stabuffer) getHeaderRow(labels []string, includeVersions bool) []string {
	numcols := 2 + (len(sb) << 2)
	row := make([]string, numcols, numcols)
	setTimeHeaders(row)
	versionStr := ""
	for i, label := range labels {
		offset := 2 + (i << 2)
		if includeVersions {
			versionStr = fmt.Sprintf(", ver. %d", <-sb[i].verc)
		}
		row[offset+0] = fmt.Sprintf("%s%s (Min)", label, versionStr)
		row[offset+1] = fmt.Sprintf("%s%s (Mean)", label, versionStr)
		row[offset+2] = fmt.Sprintf("%s%s (Max)", label, versionStr)
		row[offset+3] = fmt.Sprintf("%s%s (Count)", label, versionStr)
	}
	return row
}

type rawbufentry struct {
	rawc chan btrdb.RawPoint
	verc chan uint64
	errc chan error
	pt   btrdb.RawPoint
	open bool
}

// rawbuffer satisfies the streambuffer interface using raw points
type rawbuffer []rawbufentry

func (rb rawbuffer) getTime(i int) int64 {
	return rb[i].pt.Time
}

func (rb rawbuffer) isOpen(i int) bool {
	return rb[i].open
}

func (rb rawbuffer) readPoint(i int) (bool, error) {
	rb[i].pt, rb[i].open = <-rb[i].rawc
	if !rb[i].open {
		err := <-rb[i].errc
		return rb[i].open, err
	}
	return rb[i].open, nil
}

func (rb rawbuffer) writePoint(i int, row []string) {
	offset := 2 + i
	row[offset] = fmt.Sprintf("%f", rb[i].pt.Value)
}

func (rb rawbuffer) writeEmptyPoint(i int, row []string) {
	offset := 2 + i
	row[offset] = ""
}

func (rb rawbuffer) getHeaderRow(labels []string, includeVersions bool) []string {
	numcols := 2 + len(rb)
	row := make([]string, numcols, numcols)
	setTimeHeaders(row)
	versionStr := ""
	for i, label := range labels {
		offset := 2 + i
		if includeVersions {
			versionStr = fmt.Sprintf(", ver. %d", <-rb[i].verc)
		}
		row[offset+0] = fmt.Sprintf("%s%s", label, versionStr)
	}
	return row
}

// MakeCSVQuery performs a CSV query, and outputs the result using the provided
// CSV writer.
func MakeCSVQuery(ctx context.Context, b *btrdb.BTrDB, q *CSVQuery, w *csv.Writer) error {
	var numstreams = len(q.Streams)
	if numstreams != len(q.Labels) {
		return fmt.Errorf("Got %d streams but %d labels", len(q.Streams), len(q.Labels))
	}

	var versions = q.Versions
	if versions == nil {
		versions = make([]uint64, numstreams, numstreams)
	}

	switch q.QueryType {
	case AlignedWindowsQuery:
		var sq stabuffer = make([]stabufentry, numstreams, numstreams)
		for i, s := range q.Streams {
			sq[i].stac, sq[i].verc, sq[i].errc = s.AlignedWindows(ctx, q.StartTime, q.EndTime, q.Depth, versions[i])
		}
		return createCSV(sq, q, w, true)
	case WindowsQuery:
		var sq stabuffer = make([]stabufentry, numstreams, numstreams)
		for i, s := range q.Streams {
			sq[i].stac, sq[i].verc, sq[i].errc = s.Windows(ctx, q.StartTime, q.EndTime, q.WindowSize, q.Depth, versions[i])
		}
		return createCSV(sq, q, w, true)
	case RawQuery:
		var sq rawbuffer = make([]rawbufentry, numstreams, numstreams)
		for i, s := range q.Streams {
			sq[i].rawc, sq[i].verc, sq[i].errc = s.RawValues(ctx, q.StartTime, q.EndTime, versions[i])
		}
		return createCSV(sq, q, w, false)
	default:
		return errors.New("Invalid query type")
	}
}

func createCSV(buf streambuffer, q *CSVQuery, w *csv.Writer, statistical bool) error {
	// Buffer for the row of the CSV that we are writing
	var row = buf.getHeaderRow(q.Labels, q.IncludeVersions)

	// Write the header row
	var err = w.Write(row)
	if err != nil {
		return err
	}

	var open bool
	var numopen = len(q.Streams)
	for i := range q.Streams {
		open, err = buf.readPoint(i)
		if !open {
			numopen--
			if err != nil {
				return err
			}
		}
	}

	for numopen != 0 {
		// Compute the time of the next row
		var earliest int64 = math.MaxInt64
		for i := range q.Streams {
			if buf.isOpen(i) && buf.getTime(i) < earliest {
				earliest = buf.getTime(i)
			}
		}

		// Compute the next row
		row[0] = fmt.Sprintf("%d", earliest)
		row[1] = time.Unix(0, earliest).Format("2006-01-02 15:04:05.000000000")
		for i := range q.Streams {
			if !buf.isOpen(i) {
				continue
			} else if buf.getTime(i) == earliest {
				buf.writePoint(i, row)

				// We consumed this point, so fetch the next point
				open, err = buf.readPoint(i)
				if !open {
					numopen--
					if err != nil {
						return err
					}
				}
			} else {
				buf.writeEmptyPoint(i, row)
			}
		}

		// Emit the row
		err = w.Write(row)
		if err != nil {
			return nil
		}
	}

	return nil
}
