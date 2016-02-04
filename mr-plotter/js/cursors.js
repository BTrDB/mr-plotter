// Code to describe the behavior of cursors

function init_cursors(self) {
    self.idata.horizCursor1 = undefined;
    self.idata.horizCursor2 = undefined;
    self.idata.vertCursor1 = undefined;
    self.idata.vertCursor2 = undefined;
    self.idata.showingVertCursors = false;
    self.idata.showingHorizCursors = false;
}

/* d3chartgroup is a d3 selection. updateCallback is a function to call when the position of this cursor is updated. */
function Cursor(self, coord, d3chartgroup, length, offset, vertical, $background, updateCallback) {
    this.s3ui_instance = self;
    this.coord = coord;
    coord--;
    if (vertical) {
        this.rectMarker = d3chartgroup.append("rect")
            .attr("x", coord - 1)
            .attr("y", offset)
            .attr("width", 3)
            .attr("height", length)
            .attr("fill-opacity", 1)
            .attr("class", "vertCursor")
          .node();
    } else {
        this.rectMarker = d3chartgroup.append("rect")
            .attr("x", offset)
            .attr("y", coord - 1)
            .attr("width", length)
            .attr("height", 3)
            .attr("fill-opacity", 1)
            .attr("class", "horizCursor")
          .node();
    }
    this.parent = d3chartgroup.node();
    this.vertical = vertical;
    this.selected = false;
    var cursorObj = this;
    this.$background = $background;
    this.callback = updateCallback;
    $(this.rectMarker).on("mousedown.cursor", function (event) {
            cursorObj.select(vertical ? event.pageX : event.pageY);
        });
}

Cursor.prototype.updateLength = function (newLength) {
    if (this.vertical) {
        this.rectMarker.setAttribute("height", newLength);
    } else {
        this.rectMarker.setAttribute("width", newLength);
    }
}

Cursor.prototype.updateCoordinate = function (newCoord) {
    this.coord = newCoord;
    if (this.vertical) {
        this.rectMarker.setAttribute("x", newCoord - 1);
    } else {
        this.rectMarker.setAttribute("y", newCoord - 1);
    }
}

Cursor.prototype.select = function (initCoord) {
    this.selected = true;
    this.rectMarker.setAttribute("fill-opacity", 0.5);
    var intermediateCoord = this.coord;
    var cursorObj = this;
    cursorObj.$background.css("cursor", this.vertical ? "col-resize" : "row-resize");
    $(document).on("mousemove.cursor", function (event) {
            var attr, eventVal;
            if (cursorObj.vertical) {
                attr = "x";
                eventVal = event.pageX;
            } else {
                attr = "y";
                eventVal = event.pageY;
            }
            intermediateCoord = cursorObj.coord + (eventVal - initCoord);
            cursorObj.rectMarker.setAttribute(attr, intermediateCoord - 1);
        });
    $(document).on("mouseup.cursor", function (event) {
            if (intermediateCoord < 0 || intermediateCoord >= (cursorObj.vertical ? cursorObj.s3ui_instance.idata.WIDTH : cursorObj.s3ui_instance.idata.HEIGHT)) {
                cursorObj.deleteSelf();
            } else {
                cursorObj.coord = intermediateCoord;
                cursorObj.deselect();
            }
        });
}

Cursor.prototype.deselect = function () {
    this.selected = false;
    this.$background.css("cursor", "");
    this.rectMarker.setAttribute("fill-opacity", 1);
    $(document).off(".cursor");
    this.callback();
}

Cursor.prototype.deleteSelf = function () {
    if (this.selected) {
        $(document).off(".cursor");
        this.$background.css("cursor", "");
    }
    $(this.rectMarker).off(".cursor");
    if (this.vertical) {
        if (this.s3ui_instance.idata.vertCursor1 == this) {
            this.s3ui_instance.idata.vertCursor1 = undefined;
        } else {
            this.s3ui_instance.idata.vertCursor2 = undefined;
        }
    } else {
        if (this.s3ui_instance.idata.horizCursor1 == this) {
            this.s3ui_instance.idata.horizCursor1 = undefined;
        } else {
            this.s3ui_instance.idata.horizCursor2 = undefined;
        }
    }
    this.parent.removeChild(this.rectMarker);
    this.callback();
}

function updateVertCursorStats(self) {
    if (self.idata.initialized && self.idata.oldDomain != undefined) {
        var cursors = self.idata.cursorDataElems;
        var scale = self.idata.oldXScale;
        var firstCursor = self.idata.vertCursor1;
        var secondCursor = self.idata.vertCursor2;
        if (firstCursor == undefined && secondCursor == undefined) {
            hideEntry(cursors.x1);
            hideEntry(cursors.x2);
            hideEntry(cursors.deltax);
            hideEntry(cursors.freqx);
            hideEntry(cursors.fx1);
            hideEntry(cursors.fx2);
            self.idata.showingVertCursors = false;
            if (!self.idata.showingHorizCursors) {
                shrinkMarginIfNecessary(self);
            }
            return;
        } else if (firstCursor == undefined) {
            firstCursor = secondCursor;
            secondCursor = undefined;
        } else if (secondCursor != undefined && firstCursor.coord > secondCursor.coord) {
            secondCursor = firstCursor;
            firstCursor = self.idata.vertCursor2;
        }
        self.idata.showingVertCursors = true;
        growMarginIfNecessary(self);
        showEntry(cursors.x1);
        var domain = scale.domain();
        var x1date, x1millis, x1nanos;
        var x2date, x2millis, x2nanos;
        pixelwidthnanos = (domain[1] - domain[0]) / self.idata.WIDTH * 1000000;
        var arr = getScaleTime(firstCursor, scale, pixelwidthnanos);
        x1date = arr[0];
        x1millis = arr[1];
        x1nanos = arr[2];
        var x1millisextra = x1millis >= 0 ? x1millis % 1000 : ((x1millis % 1000) + 1000);
        cursors.x1[1].innerHTML = self.idata.labelFormatter.format(x1date) + "." + (1000 + x1millisextra).toString().slice(1) + (1000000 + x1nanos).toString().slice(1);
        if (secondCursor == undefined) {
            hideEntry(cursors.x2);
            hideEntry(cursors.deltax);
            hideEntry(cursors.freqx);
        } else {
            arr = getScaleTime(secondCursor, scale, pixelwidthnanos);
            x2date = arr[0];
            x2millis = arr[1];
            x2nanos = arr[2];
            var x2millisextra = x2millis >= 0 ? x2millis % 1000 : ((x2millis % 1000) + 1000);
            showEntry(cursors.x2);
            cursors.x2[1].innerHTML = self.idata.labelFormatter.format(x2date) + "." + (1000 + x2millisextra).toString().slice(1) + (1000000 + x2nanos).toString().slice(1);
            var millidiff = x2millis - x1millis;
            var nanodiff = x2nanos - x1nanos;
            if (nanodiff < 0) {
                nanodiff += 1000000;
                millidiff--;
            }
            nanodiff = s3ui.timeToStr([millidiff, nanodiff]);
            showEntry(cursors.deltax);
            cursors.deltax[1].innerHTML = nanodiff;
            showEntry(cursors.freqx);
            cursors.freqx[1].innerHTML = (1000 / (x2millis - x1millis + ((x2nanos - x1nanos) / 1000000)));
        }
        if (self.idata.showingDensity != undefined && self.idata.oldData.hasOwnProperty(self.idata.showingDensity)) {
            x1millis -= self.idata.offset; // switch to UTC time
            x2millis -= self.idata.offset; // switch to UTC time
            var selectedData = self.idata.oldData[self.idata.showingDensity][1];
            var pwedelta = self.idata.oldData[self.idata.showingDensity][2] - 1; // the exponent for the width of the range
            var delta = Math.pow(2, pwedelta);
            var deltamillis = Math.floor(delta / 1000000);
            var deltananos = delta % 1000000;
            var timearr;
            if (selectedData.length > 0) {
                var units = self.idata.oldData[self.idata.showingDensity][0].Properties.UnitofMeasure;
                var leftPoint = getNearestDataPoint(self, x1millis, x1nanos, selectedData, self.idata.showingDensity, delta * 2);
                showEntry(cursors.fx1);
                if (leftPoint.length == 6) { // if we haven't cached the exact time
                    timearr = [leftPoint[0], leftPoint[1]];
                    timearr[1] += deltananos;
                    timearr[0] += deltamillis;
                    if (timearr[1] >= 1000000) {
                        timearr[1] -= 1000000;
                        timearr[0] += 1;
                    }
                    cursors.fx1[1].innerHTML = s3ui.timeToStr(timearr) + " \xB1 2";
                    showExp(self, cursors.fx1[2]);
                    cursors.fx1[2].innerHTML = pwedelta;
                } else {
                    timearr = [leftPoint[6], leftPoint[7]];
                    cursors.fx1[1].innerHTML = s3ui.timeToStr(timearr);
                    hideExp(cursors.fx1[2]);
                }
                cursors.fx1[3].innerHTML = leftPoint[3].toPrecision(15) + " " + units;
                if (secondCursor == undefined) {
                    hideEntry(cursors.fx2);
                } else {
                    var rightPoint = getNearestDataPoint(self, x2millis, x2nanos, selectedData, self.idata.showingDensity, delta * 2);
                    showEntry(cursors.fx2);
                    if (rightPoint.length == 6) { // if we haven't cached the exact time
                        timearr = [rightPoint[0], rightPoint[1]];
                        timearr[1] += deltananos;
                        timearr[0] += deltamillis;
                        if (timearr[1] >= 1000000) {
                            timearr[1] -= 1000000;
                            timearr[0] += 1;
                        }
                        cursors.fx2[1].innerHTML = s3ui.timeToStr(rightPoint) + " \xB1 2";
                        showExp(self, cursors.fx2[2]);
                        cursors.fx2[2].innerHTML = pwedelta;
                    } else {
                        timearr = [rightPoint[6], rightPoint[7]];
                        cursors.fx2[1].innerHTML = s3ui.timeToStr(timearr);
                        hideExp(cursors.fx2[2]);
                    }
                    cursors.fx2[3].innerHTML = rightPoint[3].toPrecision(15) + " " + units;
                }
            }
        } else {
            hideEntry(cursors.fx1);
            hideEntry(cursors.fx2);
        }
    }
}

function hideExp(elem) {
    elem.style["font-size"] = "1px";
    elem.style["font-color"] = "none";
    elem.innerHTML = ",";
}

function showExp(self, elem) {
    elem.style["font-size"] = self.idata.scriptsize;
    elem.style["font-color"] = "black";
}

function updateHorizCursorStats(self) {
    if (self.idata.initialized) {
        var cursors = self.idata.cursorDataElems;
        var firstCursor = self.idata.horizCursor1;
        var secondCursor = self.idata.horizCursor2;
        if (self.idata.showingDensity == undefined || !self.idata.oldData.hasOwnProperty(self.idata.showingDensity) || (firstCursor == undefined && secondCursor == undefined)) {
            hideEntry(cursors.y1);
            hideEntry(cursors.y2);
            hideEntry(cursors.deltay);
            self.idata.showingHorizCursors = false;
            if (!self.idata.showingVertCursors) {
                shrinkMarginIfNecessary(self);
            }
            return;
        } else if (firstCursor == undefined) {
            firstCursor = secondCursor;
            secondCursor = undefined;
        } else if (secondCursor != undefined && firstCursor.coord < secondCursor.coord) {
            secondCursor = firstCursor;
            firstCursor = self.idata.horizCursor2;
        }
        self.idata.showingHorizCursors = true;
        growMarginIfNecessary(self);
        var scale = self.idata.oldAxisData[self.idata.streamSettings[self.idata.showingDensity].axisid][2];
        var units = self.idata.oldData[self.idata.showingDensity][0].Properties.UnitofMeasure;
        var firstVal = scale.invert(firstCursor.coord);
        var domain = scale.domain();
        var scaledelta = (domain[1] - domain[0]) / self.idata.HEIGHT;
        var numDigits = scaledelta % 1;
        if (numDigits == scaledelta) {
            numDigits = numDigits.toString();
            numDigits = numDigits.length - numDigits.replace(/^[0.]+/, '').length;
        } else {
            numDigits = 0;
        }
        numDigits += 1;
        showEntry(cursors.y1);
        firstVal = firstVal.toFixed(numDigits)
        cursors.y1[1].innerHTML = firstVal + " " + units;
        if (secondCursor != undefined) {
            var secondVal = scale.invert(secondCursor.coord);
            showEntry(cursors.y2);
            secondVal = secondVal.toFixed(numDigits);
            cursors.y2[1].innerHTML = secondVal + " " + units;
            showEntry(cursors.deltay);
            cursors.deltay[1].innerHTML = (secondVal - firstVal).toFixed(numDigits) + " " + units;
        } else {
            hideEntry(cursors.y2);
            hideEntry(cursors.deltay);
        }
    }
}

/* PIXELWIDTHNANOS is (scale.domain()[1] - scale.domain()[0]) / self.idata.WIDTH * 1000000.
   It is a parameter for the sake of efficiency. Returns an array of the form
   [date obj, milliseconds, nanoseconds] which represents the time on the given scale. */
function getScaleTime(cursor, scale, pixelwidthnanos) {
    var xdate = scale.invert(cursor.coord);
    var xmillis; // date converted to a number
    var xnanos = (cursor.coord - scale(xdate)) * pixelwidthnanos;
    if (xnanos < 0) {
        xnanos = Math.round(xnanos + 1000000);
        xmillis = xdate - 1;
        xdate = new Date(xmillis);
    } else {
        xmillis = xdate.getTime();
        xnanos = Math.round(xnanos);
    }
    return [xdate, xmillis, xnanos];
}

/* XMILLIS and XNANOS are the times in milliseconds and nanoseconds, in UTC time. */
function getNearestDataPoint(self, xmillis, xnanos, data, uuid, pw) {
    var xpoint = [xmillis, xnanos];
    var closestIndex = s3ui.binSearchCmp(data, xpoint, s3ui.cmpTimes);
    var currentPoint, currentDiff;
    var rivalPoint, rivalDiff;
    currentPoint = data[closestIndex];
    rivalPoint = undefined;
    if (closestIndex > 0 && s3ui.cmpTimes(currentPoint, xpoint) > 0) {
        rivalPoint = data[closestIndex - 1];
        rivalDiff = [xmillis - rivalPoint[0], xnanos - rivalPoint[1]];
        currentDiff = [currentPoint[0] - xmillis, currentPoint[1] - xnanos];
    } else if (closestIndex < data.length - 1) {
        rivalPoint = data[closestIndex + 1];
        rivalDiff = [rivalPoint[0] - xmillis, rivalPoint[1] - xnanos];
        currentDiff = [xmillis - currentPoint[0], xnanos - currentPoint[1]];
    }
    if (rivalPoint != undefined) {
        if (rivalDiff[1] < 0) {
            rivalDiff[0]--;
            rivalDiff[1] += 1000000;
        }
        if (currentDiff[1] < 0) {
            currentDiff[0]--;
            currentDiff[1] += 1000000;
        }
        if (s3ui.cmpTimes(currentDiff, rivalDiff) > 0) {
            currentPoint = rivalPoint; // cache (if necessary) and return the rival point
        }
    }
    if (currentPoint[5] == 1 && currentPoint.length == 6) {
        // Request the exact time from quasar
        var endTime = [currentPoint[0] + Math.floor(pw / 1000000), currentPoint[1] + pw % 1000000];
        if (endTime[1] >= 1000000) {
            endTime[1] -= 1000000;
            endTime[0] += 1;
        }
        var url = self.idata.dataURLStart + uuid + '?starttime=' + s3ui.timeToStr(currentPoint) + '&endtime=' + s3ui.timeToStr(endTime) + '&unitoftime=ns&pw=0';
        s3ui.getURL(url, function (data) {
                cacheExactTime(self, currentPoint, data);
            }, 'text');
    }
    return currentPoint;
}

function cacheExactTime(self, point, dataStr) {
    var receivedPoint;
    try {
        receivedPoint = JSON.parse(dataStr)[0].XReadings[0];
        point.push(receivedPoint[0]);
        point.push(receivedPoint[1]); // cache the exact time of the point at indices 6 and 7
        updateVertCursorStats(self); // to update the screen with the newly cached data
    } catch (err) {
        console.log("Bad response to request for exact point time: got " + dataStr);
    }
}

function hideEntry(entry) {
    entry[0].style.display = "none";
}

function showEntry(entry) {
    entry[0].style.display = "";
}

function getSigFigs(num) {
    var numexp = num.toString().replace(/^[0.]+/, '');
    numexp = numexp.replace(/\./, '');
    var eindex = numexp.indexOf('e');
    return eindex == -1 ? numexp.length : eindex;
}

function growMarginIfNecessary(self) {
    if (self.idata.margin.bottom == self.idata.normalbmargin) {
        self.idata.margin.bottom = self.idata.cursorbmargin;
        s3ui.updateSize(self, false);
    }
}

function shrinkMarginIfNecessary(self) {
    if (self.idata.margin.bottom == self.idata.cursorbmargin) {
        self.idata.margin.bottom = self.idata.normalbmargin;
        s3ui.updateSize(self, false);
    }
}

s3ui.init_cursors = init_cursors;
s3ui.Cursor = Cursor;
s3ui.updateVertCursorStats = updateVertCursorStats;
s3ui.updateHorizCursorStats = updateHorizCursorStats;
s3ui.hideEntry = hideEntry;
