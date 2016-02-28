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

var slashRE = new RegExp("/", "g");

function formatPath(metadata) {
    /* Check that the path doesn't contain anything nasty. */
    return escapeHTMLEntities(metadata.Path).replace(slashRE, "/ "); // so the line breaks where appropriate
}

function getFilepath(datum) {
    var rawpath = formatPath(datum);
    var sourceName = datum.Metadata.SourceName;
    return (sourceName == undefined ? '<no source name>' : sourceName) + rawpath;
}

function getInfo (datum, linebreak, escapeHTML) {
    if (linebreak == undefined) {
        linebreak = "<br>";
    }
    if (escapeHTML == undefined) {
        escapeHTML = true;
    }
    return getInfoHelper(datum, "", linebreak, escapeHTML);
}

function getInfoHelper(datum, prefix, linebreak, escapeHTML) {
    var toReturn = [];
    var toadd;
    for (var prop in datum) {
        if (datum.hasOwnProperty(prop)) {
            if (escapeHTML) {
                prop = escapeHTMLEntities(prop);
            }
            if (typeof datum[prop] == "object") {
                toReturn.push(getInfoHelper(datum[prop], prefix + prop + "/", linebreak, escapeHTML));
            } else {
                toadd = datum[prop];
                if (escapeHTML) {
                    toadd = escapeHTMLEntities(toadd);
                }
                toReturn.push(prefix + prop + ": " + toadd + linebreak);
            }
        }
    }
    return toReturn.join("");
}

function makeMenuMaker() {
    var colors = [["blue", "#0000FF"], ["red", "#FF0000"], ["green", "#008000"], ["purple", "#800080"], ["yellow green", "#9ACD32"], ["navy", "#000080"], ["maroon", "#800000"], ["fuchsia", "#FF00FF"], ["brown", "#8B4513"], ["orange", "#FFA500"], ["aqua", "#00FFFF"], ["gray", "#808080"], ["light brown", "#D2691E"], ["olive", "#808000"], ["blue violet", "#8A2BE2"], ["pink", "#FA8072"], ["lime", "#00FF00"], ["dark orange", "#FF8C00"], ["teal", "#008080"], ["yellow", "#FFFF00"], ["dark green", "#004000"], ["tan", "#C2A47C"], ["bright pink", "#FF3399"], ["dark blue", "#4682B4"]];
    var colorIndex = 0;
    return function makeColorMenu () {
        var menu = document.createElement("select");
        var option;
        for (var i = 0; i < colors.length; i++) {
            option = document.createElement("option");
            option.value = colors[i][1];
            option.innerHTML = colors[i][0];
            menu.appendChild(option);
        }
        menu.selectedIndex = colorIndex;
        colorIndex = (colorIndex + 1) % colors.length;
        return menu;
    }
}

/* Performs binary search on SORTEDLST to find the index of item whose key is
   ITEM. KEY is a function that takes an element of the list as an argument and
   returns its key. If ITEM is not the key of any of the items in SORTEDLST,
   one of the indices closest to the index where it would be is returned. */
function binSearch(sortedLst, item, key) {
    var currVal;
    var low = 0;
    var high = sortedLst.length - 1;
    var i;
    while (low < high) {
        i = Math.floor((low + high) / 2);
        currVal = key(sortedLst[i]);
        if (currVal < item) {
            low = i + 1;
        } else if (currVal > item) {
            high = i - 1;
        } else {
            return i;
        }
    }
    return low;
}

/* Performs binary search of SORTEDLST to find the index of the item that,
   according to the comparator, is equal to ITEM. COMPARATOR is a function
   that, given two elements in the array, returns a negative number of the
   first is less than the second, a positive number if it is greater, and
   zero if the two are equal. If ITEM is not equal to any of the items in
   SORTEDLST, one of the indices closes to the index where it would be is
   returned. */
function binSearchCmp(sortedLst, item, comparator) {
    var comparison;
    var low = 0;
    var high = sortedLst.length - 1;
    var i;
    while (low < high) {
        i = Math.floor((low + high) / 2);
        comparison = comparator(sortedLst[i], item);
        if (comparison < 0) {
            low = i + 1;
        } else if (comparison > 0) {
            high = i - 1;
        } else {
            return i;
        }
    }
    return low;
}

function nanosToUnit(numValue) {
    var unit;
    if (numValue >= 86400000000000) {
        numValue /= 86400000000000;
        unit = ' d';
    } else if (numValue >= 3600000000000) {
        numValue /= 3600000000000;
        unit = ' h';
    } else if (numValue >= 60000000000) {
        numValue /= 60000000000;
        unit = ' m';
    } else if (numValue >= 1000000000) {
        numValue /= 1000000000;
        unit = ' s';
    } else if (numValue >= 1000000) {
        numValue /= 1000000;
        unit = ' ms';
    } else if (numValue >= 1000) {
        numValue /= 1000;
        unit = ' us';
    } else {
        unit = ' ns';
    }
    return numValue.toFixed(3) + unit;
}

function cmpTimes(t1, t2) {
    if (t1[0] < t2[0]) {
        return -1;
    } else if (t1[0] > t2[0]) {
        return 1;
    } else if (t1[1] < t2[1]) {
        return -1;
    } else if (t1[1] > t2[1]) {
        return 1;
    } else {
        return 0;
    }
}

function timeToStr(time) {
    if (time[0] == 0) {
        return time[1].toString();
    } else {
        return time[0] + (1000000 + time[1]).toString().slice(1);
    }
}

s3ui.sanitizerdiv = document.createElement("div");
s3ui.sanitizertext = document.createTextNode("");
s3ui.sanitizerdiv.appendChild(s3ui.sanitizertext);
function escapeHTMLEntities(str) {
    s3ui.sanitizertext.textContent = str;
    return s3ui.sanitizerdiv.innerHTML;
}

function getTimezoneOffsetMinutes(tz_str, dst, getAbbrev) {
    var janDate = new timezoneJS.Date(2014, 0, 1, tz_str);
    var junDate = new timezoneJS.Date(2014, 5, 1, tz_str);
    var janOffset = janDate.getTimezoneOffset();
    var junOffset = junDate.getTimezoneOffset();
    if (dst) {
        if (getAbbrev) {
            if (janOffset < junOffset) {
                return [janOffset, janDate.getTimezoneAbbreviation()];
            } else {
                return [junOffset, junDate.getTimezoneAbbreviation()];
            }
        }
        return Math.min(janDate.getTimezoneOffset(), junDate.getTimezoneOffset());
    } else {
        if (getAbbrev) {
            if (janOffset > junOffset) {
                return [janOffset, janDate.getTimezoneAbbreviation()];
            } else {
                return [junOffset, junDate.getTimezoneAbbreviation()];
            }
        }
        return Math.max(janDate.getTimezoneOffset(), junDate.getTimezoneOffset());
    }
}

function getDisplayColor(axisObj, streamSettings) {
    var streams = axisObj.streams;
    if (streams.length > 0) {
        var color = streamSettings[streams[0].uuid].color;
        for (var k = 1; k < streams.length; k++) {
            if (streamSettings[streams[k].uuid].color != color) {
                return "rgb(0, 0, 0)";
            }
        }
        return color;
    }
    return "rgb(0, 0, 0)";
}

function getUnitString(unitDict) {
    var unitList = [];
    for (unit in unitDict) {
        if (unitDict.hasOwnProperty(unit) && unitDict[unit] > 0) {
            unitList.push(s3ui.escapeHTMLEntities(unit));
        }
    }
    return unitList.join(", ");
}

s3ui.formatPath = formatPath;
s3ui.getFilepath = getFilepath;
s3ui.getInfo = getInfo;
s3ui.makeMenuMaker = makeMenuMaker;
s3ui.binSearch = binSearch;
s3ui.binSearchCmp = binSearchCmp;
s3ui.nanosToUnit = nanosToUnit;
s3ui.cmpTimes = cmpTimes;
s3ui.timeToStr = timeToStr;
s3ui.escapeHTMLEntities = escapeHTMLEntities;
s3ui.getTimezoneOffsetMinutes = getTimezoneOffsetMinutes;
s3ui.getDisplayColor = getDisplayColor;
s3ui.getUnitString = getUnitString;
