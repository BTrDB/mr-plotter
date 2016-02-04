function init_data(self) {
    /* The dataCache maps UUID to an object that maps the point width exponent to
       cached data. The array contains cached entries, objects that store a start
       time, end time, and data; the cached entries never overlap, are consolidated
       when possible, and are stored sequentially. */
    self.idata.dataCache = {};

    // The total number of data points that have been cached.
    self.idata.loadedData = 0;
    self.idata.loadedStreams = {}; // maps a stream's uuid to the total number of points that have been cached for that stream
    self.idata.lastTimes = {}; // maps a stream's uuid to the time of the last pint where there is valid data, obtained from the server
    self.idata.pollingBrackets = false; // whether or not we are periodically checking if the brackets have changed
    self.idata.bracketInterval = 5000;
    
    self.idata.dataURLStart = 'http://bunker.cs.berkeley.edu/backend/api/data/uuid/';
    
    self.idata.queryLow = 0;//-1152921504606; // in milliseconds
    self.idata.queryHigh = 3458764513820; // in milliseconds
    self.idata.pweHigh = 61;
    
    // The following fields are for rate control
    self.idata.currPWE = undefined;
    self.idata.secondaryPWE = undefined;
    self.idata.pendingSecondaryRequests = 0;
    self.idata.pendingSecondaryRequestData = {};
    self.idata.pendingRequests = 0;
}

/* The start time and end time are in MILLISECONDS, not NANOSECONDS! */
function CacheEntry(startTime, endTime, data) {
    this.start_time = startTime;
    this.end_time = endTime;
    this.cached_data = data;
}

/* Starts a cycle of polling the brackets. If already in a cycle, starts
   another cycle (which should be avoided). */
function startPollingBrackets(self) {
    self.idata.pollingBrackets = true;
    if (self.idata.selectedStreams.length > 0) {
            var uuids = self.idata.selectedStreams.map(function (s) { return s.uuid; });
            s3ui.getURL("SENDPOST " + self.idata.bracketURL + " " + JSON.stringify({"UUIDS": uuids}), function (data) {
                    pollBracket(self, uuids, data);
                });
        }
}

/* The polling system works as follows: the program polls the server for the
   bracket at specified intervals. After reading the server's response, the
   program decides if the screen is close enough to the new limit to continue
   polling. If not it stops; repaintZoomNewData (in plot.js) resumes polling
   if it detects that the stream is close enough again. This means that if the
   user scrolls to the left, one request may be made to the server even though
   the program could have avoided that. I thought that the extra computation
   due to this was small enough to ignore. */

function pollBracket(self, uuids, data) {
    var range;
    var continuePolling = true;
    try {
        range = JSON.parse(data);
    } catch (err) {
        console.log("Invalid bracket response from server");
        return;
    }
    if (range != undefined) {
        continuePolling = processBracketResponse(self, uuids, range);
    }
    if (continuePolling) {
        setTimeout(function () { startPollingBrackets(self); }, self.idata.bracketInterval);
    } else {
        self.idata.pollingBrackets = false;
    }
}

/* Trims the cache as appropriate given the server's RESPONSE as an object
   to a bracket call for a list of STREAMS. Returns true if the program
   should continue polling the server for changes in the bracket. Returns
   false if the program can stop for now. */
function processBracketResponse(self, uuids, response) {
    var prevLast, newLast;
    var domain;
    var continuePolling = false;
    var loadNewData = false;
    if (self.idata.onscreen) {
        domain = self.idata.oldXScale.domain();
    } else {
        continuePolling = true;
    }
    if (response.Brackets == undefined || response.Brackets.length != uuids.length) {
        console.log("Invalid bracket response from server: requested data for " + uuids.length + " streams, got " + JSON.stringify(response));
        return true;
    }
    for (var i = 0; i < uuids.length; i++) {
        prevLast = self.idata.lastTimes[uuids[i]];
        newLast = Math.floor(response.Brackets[i][1] / 1000000);
        if (prevLast == undefined) {
            self.idata.lastTimes[uuids[i]] = newLast;
        } else if (prevLast < newLast) {
            trimCache(self, uuids[i], prevLast);
            self.idata.lastTimes[uuids[i]] = newLast;
            if (self.idata.onscreen && (prevLast + self.idata.offset) <= domain[1] && (newLast + self.idata.offset) >= domain[0]) {
                loadNewData = true;
            }
        }
        if (self.idata.onscreen && shouldPollBrackets(self, uuids[i], domain)) {
            continuePolling = true;
        }
    }
    if (loadNewData) {
        s3ui.repaintZoomNewData(self);
    }
    return continuePolling;
}

/* Given a stream's UUID and the current domain of the graph, determines
   whether the plotter should keep polling the server for a change in
   brackets. */
function shouldPollBrackets(self, uuid, domain) {
    return !self.idata.lastTimes.hasOwnProperty(uuid) || ((domain[1] - self.idata.offset) >= (self.idata.lastTimes[uuid] + (domain[0] - domain[1])));
}

/* POINTWIDTH is the number of milliseconds in one interval. Converts this to
   the equivalent number y expressed in microseconds. Then, finds the number x
   such that 2 ^ x <= POINTWIDTH and 2 ^ (x + 1) > POINTWIDTH, and returns x.
   This replaces getResolutionStep from the older implementation that allowed
   aribtrary cutoff points returned by the server. */
function getPWExponent(pointwidth) {
    return Math.round(pointwidth * 1000000).toString(2).length - 1;
}

/* Ensures that CACHE, an array of cache entries, is not corrupted. Included
   for debugging. */
function validateCache(self, cache) {
    var currEntry;
    var invalid = false;
    var condition = 0;
    for (var i = 0; i < cache.length; i++) {
        currEntry = cache[i];
        if (currEntry == undefined) {
            invalid = true;
            condition = 1;
        } else if (currEntry.end_time <= currEntry.start_time) {
            invalid = true;
            condition = 2;
        } else if (i > 0 && currEntry.start_time <= cache[i - 1].end_time) {
            invalid = true;
            condition = 3;
        }
        if (invalid) {
            alert("CORRUPTED CACHE!!! " + condition);
            console.log(cache);
            console.log(self.idata.dataCache);
            return true;
        }
    }
    return false;
}

/* Ensures that loadedData is correct. Returns true if it is correct, and
   false if it is corrupted. This function is included for debugging. */
function validateLoaded(self) {
    var total = 0;
    var uuid;
    var pw;
    var i;
    var cache;
    var dataCache = self.idata.dataCache;
    for (uuid in dataCache) {
        if (dataCache.hasOwnProperty(uuid)) {
            for (pw in dataCache[uuid]) {
                if (dataCache[uuid].hasOwnProperty(pw)) {
                    cache = dataCache[uuid][pw];
                    for (i = 0; i < cache.length; i++) {
                        total += cache[i].cached_data.length;
                    }
                }
            }
        }
    }
    console.log(total);
    console.log(self.idata.loadedData);
    return total == self.idata.loadedData;
}

/* Checks if there are any holes in a cache entry and prints out information
   about the holes if they exist. Used for debugging. */
function validateContiguous(cacheEntry, pwe) {
    var di;
    var pw = Math.pow(2, pwe);
    for (di = 0; di < cacheEntry.cached_data.length - 1; di++) {
        if (((cacheEntry.cached_data[di + 1][0] - cacheEntry.cached_data[di][0]) * 1000000) + cacheEntry.cached_data[di + 1][1] - cacheEntry.cached_data[di][1] != pw) {
            console.log('Gap');
            console.log(((cacheEntry.cached_data[di + 1][0] - cacheEntry.cached_data[di][0]) * 1000000) + cacheEntry.cached_data[di + 1][1] - cacheEntry.cached_data[di][1]);
            console.log(pw);
            console.log(di);
        }
    }
}

/* Ensures that the stream with the specified UUID has data cached from
   STARTTIME to ENDTIME at the point width corresponding to POINTWIDTHEXP, or
   floor(lg(POINTWIDTH)). If it does not, data are procured from the server and
   added to the cache so the extent of its data is at least from STARTTIME to
   ENDTIME. STARTTIME and ENDTIME are specified in UTC (Universal Coord. Time).
   Once the data is found or procured, CALLBACK is called with an array of data
   as its first argument, where the requested data is a subset of the requested
   data (second and third arguments are the start and end time of the cache
   entry, used for the data density plot). If another call to this function is
   pending (it has requested data from the server) for a certain stream, any
   more calls for that stream will not result in a GET request (so this
   function doesn't fall behind user input). */
function ensureData(self, uuid, pointwidthexp, startTime, endTime, callback, caching) {
    pointwidthexp = Math.min(self.idata.pweHigh, pointwidthexp);
    var halfPWnanos = Math.pow(2, pointwidthexp - 1) - 1;
    var halfPWmillis = halfPWnanos / 1000000;
    startTime = Math.min(Math.max(startTime, self.idata.queryLow + Math.ceil(halfPWmillis)), self.idata.queryHigh - Math.ceil(halfPWmillis) - 1);
    endTime = Math.min(Math.max(endTime, self.idata.queryLow + Math.ceil(halfPWmillis) + 1), self.idata.queryHigh - Math.ceil(halfPWmillis));
    var dataCache = self.idata.dataCache;
    // Create the mapping for this stream if it isn't already present
    if (!dataCache.hasOwnProperty(uuid)) {
        dataCache[uuid] = {};
        self.idata.loadedStreams[uuid] = 0;
    }
    var cache;
    // Find the relevant cache, creating it if necessary
    if (dataCache[uuid].hasOwnProperty(pointwidthexp)) {
        cache = dataCache[uuid][pointwidthexp];
    } else {
        cache = [];
        dataCache[uuid][pointwidthexp] = cache;
    }
    
    var indices = getIndices(cache, startTime, endTime);
    var i = indices[0];
    var j = indices[1];
    var startsBefore = indices[2];
    var endsAfter = indices[3];
    var queryStart = startsBefore ? startTime : cache[i].end_time;
    var queryEnd = endsAfter ? endTime : cache[j].start_time;
    
    var numRequests = j - i + startsBefore + endsAfter;    
    if (numRequests == 0) {
        callback(cache[i].cached_data, cache[i].start_time, cache[i].end_time);
    } else {
        // Fetch the data between the cache entries, and consolidate into one entry
        var si = i;
        var numReceived = 0;
        var urlCallback = function (streamdata, start, end) {
                var callbackToUse;
                if (++numReceived == numRequests) {
                    callbackToUse = callback;
                } else {
                    callbackToUse = function () {};
                }
                if (dataCache.hasOwnProperty(uuid) && dataCache[uuid][pointwidthexp] == cache) { // If the stream or pointwidth has been deleted to limit memory, just return and don't cache
                    var data;
                    try {
                        data = JSON.parse(streamdata)[0].XReadings;
                    } catch (err) {
                        console.log('Invalid data response from server: ' + err);
                        // Just use the previous data that was cached for drawing
                        callback(undefined);
                        return;
                    }
                    insertData(self, uuid, cache, data, start, end, callbackToUse);
                }
            };
        
        if (numRequests == 1) {
            makeDataRequest(self, uuid, queryStart, queryEnd, pointwidthexp, halfPWnanos, urlCallback, caching);
        } else {
            if (startsBefore) {
                i--;
            }
            if (endsAfter) {
                j++;
            }
            makeDataRequest(self, uuid, queryStart, cache[i + 1].start_time, pointwidthexp, halfPWnanos, urlCallback, caching);
            for (var k = i + 1; k < j - 1; k++) {
                makeDataRequest(self, uuid, cache[k].end_time, cache[k + 1].start_time, pointwidthexp, halfPWnanos, urlCallback, caching);
            }
            makeDataRequest(self, uuid, cache[j - 1].end_time, queryEnd, pointwidthexp, halfPWnanos, urlCallback, caching);
        }
    }
}

/* Gets all the points where the middle of the interval is between queryStart
   and queryEnd, including queryStart but not queryEnd. HALFPWNANOS should be
   Math.pow(2, pointwidthexp - 1) - 1. */
function makeDataRequest(self, uuid, queryStart, queryEnd, pointwidthexp, halfpwnanos, callback, caching) {
    /* queryStart and queryEnd are the start and end of the query I want,
    in terms of the midpoints of the intervals I get back; the real archiver
    will give me back all intervals that touch the query range. So I shrink
    the range by half a pointwidth on each side to compensate for that. */
    var halfpwmillisStart = Math.floor(halfpwnanos / 1000000);
    var halfpwnanosStart = halfpwnanos - (1000000 * halfpwmillisStart);
    halfpwnanosStart = (1000000 + halfpwnanosStart).toString().slice(1);
    var url = self.idata.dataURLStart + uuid + '?starttime=' + (queryStart + halfpwmillisStart) + halfpwnanosStart + '&endtime=' + (queryEnd + halfpwmillisStart) + halfpwnanosStart + '&unitoftime=ns&pw=' + pointwidthexp;
    if (caching) {
        Meteor.call('requestData', url, function (error, data) {
                callback(data, queryStart, queryEnd);
            });
    } else {
        queueRequest(self, url, function (data) {
                callback(data, queryStart, queryEnd);
            }, 'text', pointwidthexp);
    }
}

function queueRequest(self, url, callback, datatype, pwe) {
    if (self.idata.pendingRequests == 0) {
        self.idata.currPWE = pwe;
    }
    if (self.idata.currPWE == pwe) {
        self.idata.pendingRequests++;
        s3ui.getURL(url, function (data) {
                self.idata.pendingRequests--;
                callback(data);
                if (self.idata.pendingRequests == 0) {
                    effectSecondaryRequests(self);
                }
            }, datatype, function () {
                self.idata.pendingRequests--;
                if (self.idata.pendingRequests == 0) {
                    effectSecondaryRequests(self);
                }
            });
    } else {
        if (pwe != self.idata.secondaryPWE) {
            self.idata.secondaryPWE = pwe;
            self.idata.pendingSecondaryRequests = 0;
            self.idata.pendingSecondaryRequestData = {};
        }
        self.idata.pendingSecondaryRequests++;
        var id = setTimeout(function () {
                if (self.idata.pendingSecondaryRequestData.hasOwnProperty(id)) {
                    s3ui.getURL(url, function (data) {
                            callback(data);
                        }, datatype);
                    self.idata.pendingSecondaryRequests--;
                    delete self.idata.pendingSecondaryRequestData[id];
                }
            }, 1000);
        self.idata.pendingSecondaryRequestData[id] = [url, callback, datatype];
    }
}

function effectSecondaryRequests(self) {
    if (self.idata.secondaryPWE == undefined || self.idata.pendingSecondaryRequests == 0) {
        return;
    }
    self.idata.currPWE = self.idata.secondaryPWE;
    self.idata.pendingRequests = self.idata.pendingSecondaryRequests;
    self.idata.secondaryPWE = undefined;
    var entry;
    var pendingData = self.idata.pendingSecondaryRequestData;
    for (var id in pendingData) {
        if (pendingData.hasOwnProperty(id)) {
            clearTimeout(id);
            entry = pendingData[id];
            Meteor.call("requestData", entry[0], (function (cb) {
                    return function (error, result) {
                            self.idata.pendingRequests--;
                            if (error == undefined) {
                                cb(result);
                            }
                        };
                })(entry[1]));
        }
    }
    self.idata.pendingSecondaryRequestData = {};
    self.idata.pendingSecondaryRequests = 0;
}

function insertData(self, uuid, cache, data, dataStart, dataEnd, callback) {
    var indices = getIndices(cache, dataStart, dataEnd);
    var i = indices[0];
    var j = indices[1];
    var startsBefore = indices[2];
    var endsAfter = indices[3];
    if (i == j && !startsBefore && !endsAfter) {
        callback(cache[i].cached_data, cache[i].start_time, cache[i].end_time);
        return;
    }
    var dataBefore;
    var dataAfter;
    var cacheStart;
    var cacheEnd;
    var m = 0; // the first index of data that we need
    var n = data.length; // the first index of data that we don't need, where n > m
    if (startsBefore) {
        cacheStart = dataStart;
        dataBefore = [];
    } else {
        cacheStart = cache[i].start_time;
        dataBefore = cache[i].cached_data;
        if (data.length > 0) {
            // We want to get rid of overlap
            m = s3ui.binSearch(data, cache[i].end_time, function (d) { return d[0]; });
            if (data[m][0] < cache[i].end_time) {
                m++;
            }
        }
    }
    if (endsAfter) {
        cacheEnd = dataEnd;
        dataAfter = [];
    } else {
        cacheEnd = cache[j].end_time;
        dataAfter = cache[j].cached_data;
        if (data.length > 0) {
            // We want to get rid of overlap
            n = s3ui.binSearch(data, cache[j].start_time, function (d) { return d[0]; })
            if (data[n][0] >= cache[j].start_time) {
                n--;
            }
            n++;
        }
    }
    var cacheEntry = new CacheEntry(cacheStart, cacheEnd, $.merge($.merge(dataBefore, data.slice(m, n)), dataAfter));
    var loadedStreams = self.idata.loadedStreams;
    for (var k = i; k <= j; k++) {
        self.idata.loadedData -= cache[k].cached_data.length;
        loadedStreams[uuid] -= cache[k].cached_data.length;
    }
    self.idata.loadedData += cacheEntry.cached_data.length;
    loadedStreams[uuid] += cacheEntry.cached_data.length;
    cache.splice(i, j - i + 1, cacheEntry);
    callback(cacheEntry.cached_data, cacheEntry.start_time, cacheEntry.end_time);
}

/* Given CACHE, and array of cache entries, and a STARTTIME and an ENDTIME,
   provides the necessary information to determine what data in that interval
   is not present in CACHE (where the interval includes STARTTIME but does not
   include ENDTIME). Returns a four element array. The first element is a number
   i such that STARTTIME either occurs in the cache entry at index i or between
   the cache entries at indices i - 1 and i. The second element is a number j
   such that ENDTIME either occurs in the cache entry at index j or between the
   cache entries at indices j and j + 1. The third element, a boolean, is false
   if STARTTIME occurs in the cache entry at index i and true if it is between
   the cache entries at indices i - 1 and i. The fourth element, also a boolean,
   false if ENDTIME occurs in the cache entry at index j and true if it is
   between the cache entries at indices j and j + 1 */
function getIndices(cache, startTime, endTime) {
    var startsBefore; // false if startTime starts during the cacheEntry at index i, true if it starts before
    var endsAfter; // false if endTime ends during the cacheEntry at index j, true if it ends after
    
    // Figure out whether the necessary data is in the cache
    var i, j;
    if (cache.length > 0) {
        // Try to find the cache entry with data, or determine if there is no such entry
        i = s3ui.binSearch(cache, startTime, function (entry) { return entry.start_time; });
        if (startTime < cache[i].start_time) {
            i--;
        } // Now, startTime is either in entry at index i, or between index i and i + 1, or at the very beginning
        if (i == -1) {
            // new data starts before all existing records
            i = 0;
            startsBefore = true;
        } else if (startTime <= cache[i].end_time) {
            // new data starts in cache entry at index i
            startsBefore = false;
        } else {
            // new data starts between cache entries at index i and i + 1
            startsBefore = true;
            i++; // so we don't delete the entry at index i
        }
        
        j = s3ui.binSearch(cache, endTime, function (entry) { return entry.end_time; }); // endTime is either in entry at index j, or between j - 1 and j, or between j and j + 1
        if (endTime > cache[j].end_time) {
            j++;
        } // Now, endTime is either in entry at index j, or between index j - 1 and j, or at the very end
        if (j == cache.length) {
            // new data ends after all existing records
            j -= 1;
            endsAfter = true;
        } else if (endTime >= cache[j].start_time) {
            // new data ends in cache entry at index j
            endsAfter = false;
        } else {
            // new data ends between cache entries at index j - 1 and j
            endsAfter = true;
            j--; // So we don't delete the entry at index j
        }
    } else {
        // Set variables so the first entry is created
        startsBefore = true;
        i = 0;
        
        endsAfter = true;
        j = -1;
    }
    return [i, j, startsBefore, endsAfter];
}

/* Excise the portion of the cache for the stream with UUID where the time is
   strictly greater than LASTTIME. The excising is done at all resolutions.
   LASTTIME is specified in milliseconds in Universal Coordinated Time (UTC). */
function trimCache(self, uuid, lastTime) {
    var dataCache = self.idata.dataCache;
    var data;
    if (dataCache.hasOwnProperty(uuid)) {
        var cache = dataCache[uuid];
        for (var resolution in cache) {
            if (cache.hasOwnProperty(resolution)) {
                var entries = cache[resolution];
                if (entries.length == 0) {
                    continue;
                }
                var index = s3ui.binSearch(entries, lastTime, function (entry) { return entry.start_time; });
                if (index > 0 && entries[index].start_time > lastTime && entries[index - 1].end_time > lastTime) {
                    index--;
                }
                if (entries[index].start_time <= lastTime && (data = entries[index].cached_data).length > 0) {
                    var entryIndex = s3ui.binSearch(data, lastTime, function (point) { return point[0]; });
                    if (data[entryIndex][0] <= lastTime) {
                        entryIndex++;
                    }
                    entries[index].end_time = lastTime;
                    var numpoints = data.length - entryIndex;
                    data.splice(entryIndex, numpoints);
                    self.idata.loadedData -= numpoints;
                    self.idata.loadedStreams[uuid] -= numpoints;
                    index++;
                }
                var excised = entries.splice(0, index);
                for (var i = 0; i < excised.length; i++) {
                    self.idata.loadedData -= excised[i].cached_data.length;
                    self.idata.loadedStreams[uuid] -= excised[i].cached_data.length;
                }
            }
        }
    }
}

/* Reduce memory consumption by removing some cached data. STARTTIME and
   ENDTIME are in UTC (Universal Coord. Time) and represent the extent of the
   current view (so the presently viewed data is not erased). If current memory
   consumption is less than THRESHOLD, nothing will happen; otherwise, memory
   comsumption is decreased to TARGET or lower. Returns true if memory
   consumption was decreased; otherwise, returns false. */
function limitMemory(self, streams, startTime, endTime, threshold, target) {
    if (self.idata.loadedData < threshold) {
        return false;
    }
    var dataCache = self.idata.dataCache;
    var loadedStreams = self.idata.loadedStreams;
    var currPWE = getPWExponent((endTime - startTime) / self.idata.WIDTH); // PWE stands for point width exponent
    var i, j, k;
    
    // Delete extra streams
    var uuid;
    var used;
    for (uuid in dataCache) {
        if (dataCache.hasOwnProperty(uuid)) {
            used = false;
            for (i = 0; i < streams.length; i++) {
                if (streams[i].uuid == uuid) {
                    used = true;
                    break;
                }
            }
            if (!used) {
                self.idata.loadedData -= loadedStreams[uuid];
                delete dataCache[uuid];
                delete loadedStreams[uuid];
                if (self.idata.lastTimes.hasOwnProperty(uuid)) {
                    delete self.idata.lastTimes[uuid];
                }
            }
        }
    }
    if (self.idata.loadedData <= target) {
        return true;
    }
    
    // Delete extra point width caches, if deleting streams wasn't enough
    var cache;
    var pointwidth, pointwidths;
    var pwMap = {}; // Maps uuid to 2-element array containing sorted array of pointwidths, and index of current pointwidth (if it were in the sorted array)
    for (i = 0; i < streams.length; i++) {
        cache = dataCache[streams[i].uuid];
        pointwidths = [];
        for (pointwidth in cache) {
            if (pointwidth != currPWE && cache.hasOwnProperty(pointwidth)) {
                pointwidths.push(pointwidth);
            }
        }
        pointwidths.sort(function (a, b) { return a - b; });
        j = s3ui.binSearch(pointwidths, currPWE, function (x) { return x; });
        pwMap[streams[i].uuid] = [pointwidths, j];
    }
    var remaining = true; // There are still pointwidths to remove
    var pwdata, pwcount;
    while (remaining) {
        remaining = false;
        for (i = 0; i < streams.length; i++) {
            uuid = streams[i].uuid;
            pointwidths = pwMap[uuid][0];
            j = pwMap[uuid][1];
            if (pointwidths.length != 0) {
                remaining = true;
                if (j > pointwidths.length / 2) {
                    pointwidth = pointwidths.shift();
                    j--;
                } else {
                    pointwidth = pointwidths.pop();
                }
                pwdata = dataCache[uuid][pointwidth];
                pwcount = 0;
                for (k = pwdata.length - 1; k >= 0; k--) {
                    pwcount += pwdata[k].cached_data.length;
                }
                delete dataCache[uuid][pointwidth];
                self.idata.loadedData -= pwcount;
                loadedStreams[uuid] -= pwcount;
            }
        }
        if (self.idata.loadedData <= target) {
            return true;
        }
    }
    
    // Delete extra cache entries in the current pointwidth, if deleting streams and pointwidths was not enough
    for (i = 0; i < streams.length; i++) {
        pwdata = dataCache[streams[i].uuid][currPWE];
        pwcount = 0;
        for (j = pwdata.length - 1; j >= 0; j--) {
            if ((pwdata[j].start_time <= startTime && pwdata[j].end_time >= endTime) || (pwdata[j].start_time >= startTime && pwdata[j].start_time <= endTime) || (pwdata[j].end_time >= startTime && pwdata[j].end_time <= endTime)) {
                continue; // This is the cache entry being displayed; we won't delete it
            }
            pwcount += pwdata[j].cached_data.length;
            pwdata.splice(j, 1);
        }
        self.idata.loadedData -= pwcount;
        loadedStreams[streams[i].uuid] -= pwcount;
        if (self.idata.loadedData <= target) {
            return true;
        }
    }
    
    // Delete all but displayed data, if deleting streams, pointwidths, and cache entries was not enough
    for (i = 0; i < streams.length; i++) {
        pwdata = dataCache[streams[i].uuid][currPWE][0].cached_data;
        self.idata.loadedData -= pwdata.length;
        loadedStreams[streams[i].uuid] -= pwdata.length; // this should be 0, but I'm subtracting in case there's an edge case where there are multiple cache entries left even now
        j = s3ui.binSearch(pwdata, startTime, function (d) { return d[0]; });
        k = s3ui.binSearch(pwdata, endTime, function (d) { return d[0]; });
        if (pwdata[j][0] >= startTime && j > 0) {
            j--;
        }
        if (pwdata[k][0] <= endTime && k < pwdata.length - 1) {
            k++;
        }
        dataCache[streams[i].uuid][currPWE][0] = new CacheEntry(pwdata[j][0], pwdata[k][0], pwdata.slice(j, k));
        loadedStreams[streams[i].uuid] += (k - j);
        self.idata.loadedData += (k - j);
    }
    
    // If target is still less than loadedData, it means that target isn't big enough to accomodate the data that needs to be displayed on the screen
    return true;
}

s3ui.init_data = init_data;
s3ui.startPollingBrackets = startPollingBrackets;
s3ui.processBracketResponse = processBracketResponse;
s3ui.shouldPollBrackets = shouldPollBrackets;
s3ui.getPWExponent = getPWExponent;
s3ui.ensureData = ensureData;
s3ui.limitMemory = limitMemory;
