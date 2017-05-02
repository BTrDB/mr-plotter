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

	btrdb "gopkg.in/btrdb.v4"
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
}

type streamquery struct {
	rawc chan btrdb.RawPoint
	stac chan btrdb.StatPoint
	verc chan uint64
	errc chan error
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

	/* State for each stream. */
	var sq = make([]streamquery, numstreams, numstreams)

	switch q.QueryType {
	case AlignedWindowsQuery:
		for i, s := range q.Streams {
			sq[i].stac, sq[i].verc, sq[i].errc = s.AlignedWindows(ctx, q.StartTime, q.EndTime, q.Depth, versions[i])
		}
		return createStatisticalCSV(sq, q, w)
	case WindowsQuery:
		for i, s := range q.Streams {
			sq[i].stac, sq[i].verc, sq[i].errc = s.Windows(ctx, q.StartTime, q.EndTime, q.WindowSize, q.Depth, versions[i])
		}
		return createStatisticalCSV(sq, q, w)
	case RawQuery:
		for i, s := range q.Streams {
			sq[i].rawc, sq[i].verc, sq[i].errc = s.RawValues(ctx, q.StartTime, q.EndTime, versions[i])
		}
	default:
		return errors.New("Invalid query type")
	}

	return nil
}

type statbufentry struct {
	pt   btrdb.StatPoint
	open bool
}

func createStatisticalCSV(sq []streamquery, q *CSVQuery, w *csv.Writer) error {
	var numstreams = len(sq)
	var numcols = 2 + (numstreams << 2)

	// Buffer for the row of the CSV that we are writing
	var row = make([]string, numcols, numcols)

	// Write the header row
	row[0] = "Timestamp (ns)"
	row[1] = "Date/Time"
	for i, label := range q.Labels {
		offset := 2 + (i << 2)
		row[offset+0] = fmt.Sprintf("%s (Min)", label)
		row[offset+1] = fmt.Sprintf("%s (Mean)", label)
		row[offset+2] = fmt.Sprintf("%s (Max)", label)
		row[offset+3] = fmt.Sprintf("%s (Count)", label)
	}

	var err = w.Write(row)
	if err != nil {
		return err
	}

	var buf = make([]statbufentry, numstreams, numstreams)
	var numopen = numstreams
	for i := range buf {
		buf[i].pt, buf[i].open = <-sq[i].stac
		if !buf[i].open {
			numopen--
			if err = <-sq[i].errc; err != nil {
				return err
			}
		}
	}

	for {
		// Compute the time of the next row
		var earliest int64 = math.MaxInt64
		for i := range buf {
			if buf[i].open && buf[i].pt.Time < earliest {
				earliest = buf[i].pt.Time
			}
		}

		// Compute the next row
		row[0] = fmt.Sprintf("%d", earliest)
		row[1] = time.Unix(0, earliest).Format(time.RFC3339Nano)
		for i := range buf {
			offset := 2 + (i << 2)
			if !buf[i].open {
				continue
			} else if buf[i].pt.Time == earliest {
				row[offset+0] = fmt.Sprintf("%f", buf[i].pt.Min)
				row[offset+1] = fmt.Sprintf("%f", buf[i].pt.Mean)
				row[offset+2] = fmt.Sprintf("%f", buf[i].pt.Max)
				row[offset+3] = fmt.Sprintf("%d", buf[i].pt.Count)

				// We consumed this point, so fetch the next point
				buf[i].pt, buf[i].open = <-sq[i].stac
				if !buf[i].open {
					numopen--
					if err = <-sq[i].errc; err != nil {
						return err
					}
				}
			} else {
				row[offset+0] = ""
				row[offset+1] = ""
				row[offset+2] = ""
				row[offset+3] = ""
			}
		}

		if numopen == 0 {
			break
		}

		// Emit the row
		err = w.Write(row)
		if err != nil {
			return nil
		}
	}

	return nil
}
