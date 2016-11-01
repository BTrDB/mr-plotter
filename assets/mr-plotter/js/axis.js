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

// Functions and state of the selection of axes

function Axis (id, fixedaxis) {
    this.axisname = id;
    this.axisid = id;
    this.truename = id;
    this.streams = [];
    this.units = {};
    this.autoscale = true;
    this.newaxis = true;
    this.manualscale = [-1, 1];
    this.right = false; // true if this axis is to be displayed on the right side of the graph


    // PSL wants the plotter to start with "fixed axes" that can't be changed
    this.fixedaxis = fixedaxis || false;
}

function init_axis(self) {
    /* These three variables are maintained with every operation. */
    self.idata.yAxes = []; // Stores a list of axis objects
    self.idata.numAxes = 0; // The number of times "Add a Y-Axis" has been clicked plus 1, NOT the number of axes
    self.idata.axisMap = {}; // Maps the id of an axis to its axis object
}

/* Move STREAM from one axis to another. If FROMAXISID or TOAXISID is null,
   STREAM is simply removed from FROMAXISID or added to TOAXISID.
   This updates the "Axes" box, but not the select menu in the "Legend" box. */
function changeAxis(self, stream, fromAxisID, toAxisID, updateGraph) {
    var unit = stream.Properties.UnitofMeasure;
    if (fromAxisID != null) {
        var streamList = self.idata.axisMap[fromAxisID].streams;
        for (var i = 0; i < streamList.length; i++) {
            if (streamList[i].uuid == stream.uuid) {
                streamList.splice(i, 1);
                break;
            }
        }
        self.idata.axisMap[fromAxisID].units[unit]--;
        updateYAxis(self, fromAxisID);
    }
    if (toAxisID != null) {
        self.idata.axisMap[toAxisID].streams.push(stream);
        var unitDict = self.idata.axisMap[toAxisID].units;
        if (unitDict.hasOwnProperty(unit)) {
            unitDict[unit]++;
        } else {
            unitDict[unit] = 1;
        }
        self.idata.streamSettings[stream.uuid].axisid = toAxisID;
        updateYAxis(self, toAxisID);
    }
}

/* Create a new y-axis, updating the variables and the screen as necessary. */
function addYAxis(self, fixedaxis) {
    fixedaxis = fixedaxis || false;

    var id = "y" + (++self.idata.numAxes);
    var axisObj = new Axis(id, fixedaxis);
    self.idata.yAxes.push(axisObj);
    self.idata.axisMap[id] = axisObj;
    var row = d3.select(self.find("tbody.axes"))
      .append("tr")
        .attr("class", "axissetting axis-" + id);
    var axisname = row.append("td")
      .attr("style","width: 100px;")
      .append("input")
        .attr("type", "text")
        .attr("class", "axisname form-control thin-margin-text")
        .attr("value", id)
        .node();
    axisname.onchange = function () {
                var newname = s3ui.escapeHTMLEntities(this.value); // to prevent HTML or Javascript injection
                axisObj.axisname = newname;
                axisObj.truename = this.value; // use this version in Permalinks so axis names aren't escaped twice
                self.$("option.option-" + axisObj.axisid).html(newname);
                self.$("text.axistitle-" + axisObj.axisid).html(newname);
            };

    if (fixedaxis) {
        axisname.setAttribute("readonly", "readonly");
    }

    row.append("td")
        .attr("class", "axisstreams")
        .attr("style", "width: 200px; height: 15px; line-height: 15px; overflow: hidden;");
    row.append("td")
        .attr("style","width: 50px;")
        .attr("class", "axisunits");

    // Create the DOM element for selecting the range




    // div4 = sideElem.append("div")
        // .attr("style", "width: 100px;");

    // var rangeElem = div4.append("div")
        // .attr("class", "btn btn-info autoscalebutton")
        // .html("Autoscale")
        // .node();


    // var rangeboxElem = row.append("td");

    // AUTOSCALE RANGE
    var rangeRow = document.createElement("td");

    // rangeRow.attr("style","float: right;");

    var selectElem = d3.select(rangeRow).append("div")
        .attr("class", "axisrangeselect form-inline");

    selectElem.append("span")
        .html("<div style='line-height: 2px; margin-left: 10px;'>&nbsp;</div>");

    var leftBox = selectElem.append("input")
        .attr("type", "text")
        .attr("class", "axisrange form-control thin-margin-text")
        .attr("style", "margin-left: 0; width: 44px; text-align: center;")
        .node();
    leftBox.onchange = function () {
                axisObj.manualscale[0] = parseFloat(this.value.trim());
                s3ui.applySettings(self, false);
            };
    axisObj.leftBox = leftBox;

    selectElem.append("span")
        .text(" to ");

    var rightBox = selectElem.append("input")
        .attr("type", "text")
        .attr("class", "axisrange form-control thin-margin-text")
        .attr("style", "width: 44px; text-align: center;")
        .node();
    rightBox.onchange = function () {
                axisObj.manualscale[1] = parseFloat(this.value.trim());
                s3ui.applySettings(self, false);
            };
    axisObj.rightBox = rightBox;





    var settingsElem = row.append("td")
        .attr("style", "width: 50px;")
        .append("table")
        .attr("class", "axissettingtable");

    var sideElem = settingsElem.append("tr")
      .append("td")
      .attr("class", "settings-control")
      .append("div")
        .attr("class", "btn-group axisside")
        .attr("style", "width: 100px; display: block;")
        .attr("data-toggle", "buttons");


        // GLYPHS AND BUTTONS
    var div = sideElem.append("label")
        .attr("class", "btn btn-info active")
        .attr("style", "width: 26px; border-radius: 4px; margin-left: 4px;");

    div.append("input")
        .attr("type", "radio")
        .attr("name", "side-" + id + "i" + self.idata.instanceid)
        .attr("checked", true)
        .node().onchange = function () {
                if (axisObj.right !== false) {
                    axisObj.right = false;
                    s3ui.applySettings(self, false);
                }
            };
    div.append("span")
        .attr("class", "glyphicon glyphicon-arrow-left");



    div = sideElem.append("label")
        .attr("class", "btn btn-info")
        .attr("style", "width: 26px; border-radius: 4px; margin-left: 4px;");
    div.append("input")
        .attr("type", "radio")
        .attr("name", "side-" + id + "i" + self.idata.instanceid)
        .node().onchange = function () {
                if (axisObj.right !== null) {
                    axisObj.right = null;
                    s3ui.applySettings(self, false);
                }
            };
    div.append("span")
        .attr("class", "glyphicon glyphicon-remove");


    div = sideElem.append("label")
        .attr("class", "btn btn-info")
        .attr("style", "width: 26px; border-radius: 4px; margin-left: 4px;");
    div.append("input")
        .attr("type", "radio")
        .attr("name", "side-" + id + "i" + self.idata.instanceid)
        .node().onchange = function () {
                if (axisObj.right !== true) {
                    axisObj.right = true;
                    s3ui.applySettings(self, false);
                }
            };
    div.append("span")
        .attr("class", "glyphicon glyphicon-arrow-right");


    // AUTOSCALE BUTTON

        // var settingsElem = row.append("td")
        // .append("table")
        // .attr("class", "axissettingtable")
        // .attr("style", "width: 240px");

    var scalingElem = row.append("td")
        .attr("style", "width: 50px;");

    var div2 = scalingElem.append("div");

    var rangeElem = div2.append("div")
        .attr("class", "btn btn-info autoscalebutton")
        .html("&varr; Autoscale &varr;")
        .node();

    rangeElem.onclick = function () {
                axisObj.autoscale = true;
                s3ui.applySettings(self, false);
            };
    var thisRow = rangeElem.parentNode.parentNode;
    thisRow.parentNode.appendChild(rangeRow, thisRow.nextSibling);
    axisObj.rangeRow = rangeRow;


    // REMOVE BUTTON ROW
    if (!fixedaxis) {
        var removeButton = row.append("td")
            .html("X")
            .attr('onclick', 'clicked()')
            .attr("class", "removebutton btn btn-danger")
            .attr("style", "margin-top: 6px; margin-left: 10px; padding: 3px 6px 2px 6px; font-size: 12px;")
            .node().onclick = function () {
                    removeYAxis(self, axisObj);
                };
    }

    d3.selectAll(self.$("select.axis-select"))
      .append("option")
        .attr("class", "option-" + axisObj.axisid)
        .attr("value", axisObj.axisid)
        .html(axisObj.axisname);
    return id;
}

/* Delete the y-axis specified by the Axis object AXIS. */
function removeYAxis(self, axis) {
    if (self.idata.yAxes.length == 1) {
        return; // The user can't remove the last axis
    }
    var moveTo = axis.axisid == self.idata.yAxes[0].axisid ? 1 : 0;
    var streamList = axis.streams;
    var i;
    var selectbox;
    var mustUpdate = (streamList.length > 0);
    for (i = streamList.length - 1; i >= 0; i--) {
        selectbox = self.find(".axis-select-" + streamList[i].uuid);
        selectbox.selectedIndex = moveTo;
        selectbox.onchange(null, true); // We update the graph ONCE at the end, not after each stream is moved
    }
    updateYAxis(self, self.idata.yAxes[moveTo].axisid);
    delete self.idata.axisMap[axis.axisid];
    var yAxes = self.idata.yAxes;
    for (i = 0; i < yAxes.length; i++) {
        if (yAxes[i] == axis) {
            yAxes.splice(i, 1);
            break;
        }
    }
    if (mustUpdate) {
        s3ui.applySettings(self, false);
    }
    var row = self.find("tr.axis-" + axis.axisid);
    row.parentNode.removeChild(row);
    self.$("option.option-" + axis.axisid).remove();
}

/* Update the list of streams and units for the axis specified by the ID
   AXISID. */
function updateYAxis (self, axisid) {
    var rowSelection = d3.select(self.find("tr.axis-" + axisid));
    var streamUpdate = rowSelection.select("td.axisstreams")
      .selectAll("div")
      .data(self.idata.axisMap[axisid].streams);
    streamUpdate.enter()
      .append("div");
    streamUpdate
        .text(function (stream) { return s3ui.formatPath(stream); });
    streamUpdate.exit()
        .remove()
    rowSelection.select("td.axisunits")
        .each(function () {
                this.innerHTML = s3ui.getUnitString(self.idata.axisMap[axisid].units);
            });
}

/* Given a stream, heuristically determines which axis (of those currently
   present) is ideal for it. Returns the index of the chosen axis in yAxes, or
   undefined if none of the current y-axes are suitable.

   The function attempts to find an axis with the same units as the stream.
   If this is not possible, it searches for an axis with no streams assigned
   to it.
   If that is not possible either, it returns undefined.

   Due to PSL's requirements, this first parses the stream name to see if it
   can identify a fixed axis for it. */
function guessYAxis(self, stream) {
    /* Logic that PSL wants me to add. */
    var axisname = s3ui.getPSLUnit(stream);
    var yAxes = self.idata.yAxes.filter(s3ui.getPSLAxisFilter(stream));
    for (var i = 0; i < yAxes.length; i++) {
        var axis = yAxes[i];
        if (axis.fixedaxis && axis.axisname === axisname) {
            return i;
        }
    }

    /* Fall back to normal logic if PSL's logic fails. */
    var axis;
    var unit = stream.Properties.UnitofMeasure;
    var backupIndex;
    for (var i = 0; i < yAxes.length; i++) {
        axisUnits = yAxes[i].units;
        if (axisUnits.hasOwnProperty(unit) && axisUnits[unit] > 0) {
            return i;
            // yAxes[i].axisname = unit;
            // yAxes[i].truename = unit;
        } else if (backupIndex == undefined && yAxes[i].streams.length == 0) {
            backupIndex = i;
            // yAxes[backupIndex].axisname = unit;
            // yAxes[backupIndex].truename = unit;
            // yAxes[i].axisname = unit;
            // console.log(yAxes[backupIndex]);
        }
    }
    return backupIndex;
}

s3ui.init_axis = init_axis;
s3ui.changeAxis = changeAxis;
s3ui.addYAxis = addYAxis;
s3ui.removeYAxis = removeYAxis;
s3ui.updateYAxis = updateYAxis;
s3ui.guessYAxis = guessYAxis;
