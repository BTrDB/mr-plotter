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

	// Labels contains the name to use for each stream in the output CSV file.
	Labels []string
}

// MakeCSVQuery performs a CSV query, and outputs the result using the provided
// CSV writer.
func MakeCSVQuery(ctx context.Context, b *btrdb.BTrDB, q *CSVQuery, w *csv.Writer) error {
	switch q.QueryType {
	case AlignedWindowsQuery:
		fallthrough
	case WindowsQuery:
		fallthrough
	case RawQuery:
		return nil
	default:
		return errors.New("Invalid query type")
	}
}
