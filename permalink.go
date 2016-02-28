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

/* This file contains the logic to validate permalink data received. The code
   that actually inserts the data into the Mongo store is not in this file. */

package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var PERMALINK_SCHEMA = map[string]map[reflect.Kind]string{
	"autoupdate": { reflect.Bool: "Boolean", },
	"axes": { reflect.Slice: "Array", },
	"resetStart": { reflect.Float64: "Number", },
	"resetEnd": { reflect.Float64: "Number", },
	"tz": { reflect.String: "String", },
	"dst": { reflect.Bool: "Boolean", },
	"streams": { reflect.Slice: "Array", },
	"window_type": { reflect.String: "String", },
	"window_width": { reflect.Float64: "Number", },
	"start": { reflect.Float64: "Number", },
	"end": { reflect.Float64: "Number", },
	"vertCursor1": { reflect.Float64: "Number", },
	"vertCursor2": { reflect.Float64: "Number", },
	"horizCursor1": { reflect.Float64: "Number", },
	"horizCursor2": { reflect.Float64: "Number", },
}

var PERMALINK_REQUIRED = []string{ "streams" }

var PERMALINK_WINDOW_CONDITIONAL = map[string][]string {
	"fixed": { "start", "end" },
	"last": { "window_width" },
	"now": { "window_width" },
}

var STREAM_SCHEMA = map[string]map[reflect.Kind]string{
	"stream": { reflect.String: "String", reflect.Map: "Object" },
	"color": { reflect.String: "String", },
	"selected": { reflect.Bool: "Boolean", },
}

var STREAM_REQUIRED = []string{ "stream" }

var AXIS_SCHEMA = map[string]map[reflect.Kind]string{
	"axisname": { reflect.String: "String", },
	"streams": { reflect.Slice: "Array", },
	"scale": { reflect.Slice: "Array", },
	"rightside": { reflect.Map: "Object", reflect.Bool: "Boolean" },
}

var AXIS_REQUIRED = []string{ "axisname", "streams", "scale", "rightside" }

/* JSONPERMALINK is JSON that has been parsed into a map. This function verifies
   its schema (so that users can't insert arbitrary JSON into the database. */
func validatePermalinkJSON(jsonPermalink map[string]interface{}) error {
	var err error
	var i, j int
	var ok bool
	var streamint interface{}
	var stream map[string]interface{}
	var streams []interface{}
	var streamcolor string
	var axisint interface{}
	var axis map[string]interface{}
	var axes []interface{}
	var rightside interface{}
	var axisstream interface{}
	var axisscale []interface{}
	var windowtype string
	var additionalproperties []string
	
	var axisscaleOK bool
	
	// validate the schema
	err = permalinkCheckExtraFields(jsonPermalink, PERMALINK_SCHEMA)
	if err != nil {
		return err
	}
	
	// check that required fields are present
	err = permalinkCheckRequiredFields(jsonPermalink, PERMALINK_REQUIRED)
	if err != nil {
		return err
	}
	
	// check that streams are valid
	streams, ok = jsonPermalink["streams"].([]interface{})
	if !ok {
		return errors.New("Error: the value corresponding to 'streams' must be an array of objects")
	}
	for i, streamint = range streams {
		if streamint == nil {
			return fmt.Errorf("Error: the element at index %d of the 'streams' array is null", i)
		}
		stream, ok = streamint.(map[string]interface{})
		if !ok {
			return fmt.Errorf("Error: the element at index %d of the 'streams' array is not an object", i)
		}
		err = permalinkCheckExtraFields(stream, STREAM_SCHEMA)
		if err != nil {
			return fmt.Errorf("%s ('streams' array, index %d)", err.Error(), i)
		}
		err = permalinkCheckRequiredFields(stream, STREAM_REQUIRED)
		if err != nil {
			return fmt.Errorf("%s ('streams' array, index %d)", err.Error(), i)
		}
		if stream["stream"] == nil {
			return fmt.Errorf("'stream' field of element at index %d of the 'streams' array is null", i)
		}
		if streamcolor, ok = stream["color"].(string); ok {
			if len(streamcolor) != 7 || streamcolor[0] != '#' {
				// This isn't a complete check, but I think it's good enough
				return fmt.Errorf("Error: stream color must be a string contining the pound sign (#) and a six digit hexademical number");
			}
		}
	}
	
	// check that axes are valid
	_, ok = jsonPermalink["axes"]
	if ok {
		axes, ok = jsonPermalink["axes"].([]interface{})
		if !ok {
			return errors.New("Error: the value corresponding to 'axes' must be an array of objects")
		}
		for i, axisint = range axes {
			if axisint == nil {
				return fmt.Errorf("Error: the element at index %d of the 'axes' array is null", i)
			}
			axis, ok = axisint.(map[string]interface{})
			if !ok {
				return fmt.Errorf("Error: the element at index %d of the 'axes' array is not an object", i)
			}
			err = permalinkCheckExtraFields(axis, AXIS_SCHEMA)
			if err != nil {
				return err
			}
			err = permalinkCheckRequiredFields(axis, AXIS_REQUIRED)
			if err != nil {
				return err
			}
			
			if rightside, ok = axis["rightside"]; ok {
				if rightside != nil {
					if _, ok = rightside.(bool); !ok {
						return fmt.Errorf("Error: rightside field of element at index %d of 'axes' may be only true, false, or null", i)
					}
				}
			}
			
			for j, axisstream = range axis["streams"].([]interface{}) {
				if _, ok = axisstream.(string); !ok {
					return fmt.Errorf("Error: element at index %d of 'streams' field of element at index %d of 'axes' field is not a string", j, i)
				}
			}
			
			/* We don't need to check if the assertion was correct, because
			   we already checked that this is a slice. */
			axisscale = axis["scale"].([]interface{})
			axisscaleOK = false
			if len(axisscale) == 2 {
				if _, ok = axisscale[0].(float64); ok {
					if _, ok = axisscale[1].(float64); ok {
						axisscaleOK = true
					}
				}
			}
			if !axisscaleOK {
				return fmt.Errorf("Error: 'scale' field of element at index %d of 'axes' field is not an array of length 2 containing numbers", i)
			}
		}
	}
	
	// check that conditional fields are present
	windowtype, ok = jsonPermalink["window_type"].(string)
	if !ok {
		windowtype = "fixed"
	}
	
	additionalproperties, ok = PERMALINK_WINDOW_CONDITIONAL[windowtype]
	if !ok {
		return fmt.Errorf("Error: %s is not a valid value for the 'window_type' field", windowtype)
	}
	err = permalinkCheckRequiredFields(jsonPermalink, additionalproperties)
	if err != nil {
		return fmt.Errorf("%s; it is required because the 'window_type' is \"%s\"", err.Error(), windowtype)
	}
	
	return nil;
}

func permalinkCheckExtraFields(object map[string]interface{}, schema map[string]map[reflect.Kind]string) error {
	for property, value := range object {
		validtypes, ok := schema[property]
		if !ok {
			return fmt.Errorf("Error: '%s' is not a valid field", property)
		} else if _, valid := validtypes[reflect.TypeOf(value).Kind()]; !valid {
			validtypeslice := make([]string, len(validtypes), len(validtypes))
			i := 0
			for _, validtype := range validtypes {
				validtypeslice[i] = validtype
				i++
			}
			return fmt.Errorf("Error: '%s' must be one of the following types: %s", property, strings.Join(validtypeslice, ", "))
		}
	}
	return nil
}

func permalinkCheckRequiredFields(object map[string]interface{}, required_properties []string) error {
	for _, property := range required_properties {
		if _, hasproperty := object[property]; !hasproperty {
			return fmt.Errorf("Error: required field '%s' is missing", property)
		}
	}
	return nil
}
