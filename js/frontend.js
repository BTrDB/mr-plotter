function init_frontend(self) {
    self.idata.streamList = [];
    self.idata.dateFormat = "%a %b %d, %Y %T";
    self.idata.dateConverter = new AnyTime.Converter({format: self.idata.dateFormat});
    self.idata.labelFormatter = new AnyTime.Converter({format: self.idata.dateFormat, utcFormatOffsetImposed: 0});
    self.idata.makeColorMenu = s3ui.makeMenuMaker();
    self.idata.streamSettings = {}; // Stores the stream settings chosen in the legend (maps uuid to a setting object)
    self.idata.selectedStreamsBuffer = self.idata.selectedStreams; // Streams that have been selected and are displayed in the legend
    self.idata.streamMessages = {}; // Maps a stream's uuid to a 2-element array containing 1) an object mapping importances (ints) to messages and 2) the importance of the current message being displayed

    self.idata.addedStreams = undefined;
    self.idata.changedTimes = undefined;
    self.idata.otherChange = undefined;
    self.idata.automaticAxisUpdate = false; // True if axes will be updated without the need for an "Update Axes" button
    self.idata.initPermalink = window.location.protocol + "//" + window.location.hostname + (function (port) { if (port === '') { return ''; } return ':' + port; })(window.location.port) + window.location.pathname + '?'; // The start of the permalink
    self.idata.loadedPermalink = false;
    self.idata.selectedLegendEntry = undefined; // The currently selected legend entry
    self.idata.chart = self.find("svg.chart");
    self.idata.widthFunction = function () {
            var $parent = $(self.find('.chartContainer'))
            var width = $parent.css("width");
            var leftpadding = $parent.css("padding-left");
            var rightpadding = $parent.css("padding-right");
            return s3ui.parsePixelsToInt(width) - s3ui.parsePixelsToInt(leftpadding) - s3ui.parsePixelsToInt(rightpadding);
        }
    self.idata.csvURL = "http://bunker.cs.berkeley.edu:9000/multicsv";
}

/* Adds or removes (depending on the value of SHOW) the stream
    described by STREAMDATA to or from the legend. UPDATE is true if
    applySettings should be immediately called after insertion or removal. */
function toggleLegend (self, show, streamdata, update) {
    if (update == undefined) {
        update = true;
    }
    self.idata.drawnBefore = false;
    var streamSettings = self.idata.streamSettings;
    var nameElem;
    if (show) {
        if (streamSettings.hasOwnProperty(streamdata.uuid) && streamSettings[streamdata.uuid].active) {
            return;
        }
        self.idata.selectedStreamsBuffer.push(streamdata);
        var row = d3.select(self.find("tbody.legend"))
          .append("tr")
            .datum(streamdata)
            .attr("class", function (d) { return "legend-" + d.uuid; });
        var colorMenu = row.append("td")
            .append(self.idata.makeColorMenu)
            .attr("class", function (d) { return "color-" + d.uuid; })
          .node();
        colorMenu.onchange = function () {
                var newColor = this[this.selectedIndex].value;
                streamSettings[streamdata.uuid].color = newColor;
                self.$("g.series-" + streamdata.uuid).attr({
                        "stroke": newColor,
                        "fill": newColor
                    });
                s3ui.applyDisplayColor(self, self.idata.axisMap[streamSettings[streamdata.uuid].axisid], streamSettings);
                self.$("polyline.density-" + streamdata.uuid).attr("stroke", newColor);
                color = [parseInt(newColor.slice(1, 3), 16), parseInt(newColor.slice(3, 5), 16), parseInt(newColor.slice(5, 7), 16)].join(", ");
                if (self.idata.selectedLegendEntry == nameElem) {
                    nameCell.style("background-color", "rgba(" + color + ", 0.3)");
                }
            };
        streamSettings[streamdata.uuid] = { color: colorMenu[colorMenu.selectedIndex].value, axisid: "y1", active: true }; // axisid is changed
        self.idata.streamMessages[streamdata.uuid] = [{0: ""}, 0];
        var color = streamSettings[streamdata.uuid].color;
        color = [parseInt(color.slice(1, 3), 16), parseInt(color.slice(3, 5), 16), parseInt(color.slice(5, 7), 16)].join(", ");
        var nameCell = row.append("td")
            .html(function (d) { return s3ui.getFilepath(d); })
            .attr("class", "streamName streamName-" + streamdata.uuid);
        nameElem = nameCell.node();
        nameElem.onclick = function () {
                if (self.idata.selectedLegendEntry == nameElem) {
                    self.idata.selectedLegendEntry = undefined;
                    setStreamMessage(self, streamdata.uuid, undefined, 4);
                    s3ui.hideDataDensity(self);
                    nameCell.style("background-color", "rgba(" + color + ", 0.1)");
                    $(self.find(".metadataDisplay")).empty();
                } else {
                    if (self.idata.selectedLegendEntry != undefined) {
                        var oldSelection = self.idata.selectedLegendEntry;
                        oldSelection.onclick(); //deselect the previous selection
                        self.idata.selectedLegendEntry = nameElem;
                        oldSelection.onmouseout();
                    } else {
                        self.idata.selectedLegendEntry = nameElem;
                    }
                    if (self.idata.oldData.hasOwnProperty(streamdata.uuid)) {
                        var xdomain = self.idata.oldXScale.domain();
                        setStreamMessage(self, streamdata.uuid, "Interval width: " + s3ui.nanosToUnit(Math.pow(2, self.idata.oldData[streamdata.uuid][2])), 4);
                    }
                    s3ui.showDataDensity(self, streamdata.uuid);
                    nameCell.style("background-color", "rgba(" + color + ", 0.3)");
                    self.find(".metadataDisplay").innerHTML = "<h3>Stream Metadata</h3>" + s3ui.getInfo(streamdata, "<br>");
                }
                s3ui.updateVertCursorStats(self);
                s3ui.updateHorizCursorStats(self);
            };
        var hovered = false;
        nameElem.onmouseover = function () {
                hovered = true;
                if (self.idata.selectedLegendEntry != nameElem) {
                    nameCell.style("background-color", "rgba(" + color + ", 0.1)");
                }
            };
        nameElem.onmouseout = function () {
                hovered = false;
                if (self.idata.selectedLegendEntry != nameElem) {
                    nameCell.style("background-color", "");
                }
            };
        var selectElem = row.append("td")
          .append("select")
            .attr("class", "axis-select form-control axis-select-" + streamdata.uuid)
            .attr("style", "padding: 0px; min-width: 4em;");
        selectElem.selectAll("option")
          .data(self.idata.yAxes)
          .enter()
          .append("option")
            .attr("class", function (d) { return "option-" + d.axisid; })
            .attr("value", function (d) { return d.axisid; })
            .html(function (d) { return d.axisname; });
        var selectNode = selectElem.node();
        var initIndex = s3ui.guessYAxis(self, streamdata); // use a heuristic to get the initial selection
        if (initIndex == undefined) {
            initIndex = self.idata.yAxes.length;
            s3ui.addYAxis(self);
        }
        selectNode.selectedIndex = initIndex;
        selectNode.setAttribute("data-prevselect", selectNode[selectNode.selectedIndex].value);
        selectNode.onchange = function (event, suppressUpdate) {
                var newID = this[this.selectedIndex].value;
                s3ui.changeAxis(self, streamdata, this.getAttribute("data-prevselect"), newID);
                this.setAttribute("data-prevselect", newID);
                if (suppressUpdate == undefined) {
                    s3ui.applySettings(self, false);
                }
            };
        var intervalWidth = row.append("td").attr("class", "message-" + streamdata.uuid).node();
        s3ui.changeAxis(self, streamdata, null, selectNode[selectNode.selectedIndex].value);
        $("select.color-" + streamdata.uuid).simplecolorpicker({picker: true});
        if (update) { // Go ahead and display the stream
            s3ui.applySettings(self, true);
        }
    } else {
        if (!streamSettings.hasOwnProperty(streamdata.uuid) || !streamSettings[streamdata.uuid].active) {
            return;
        }
        nameElem = self.idata.selectedLegendEntry;
        if (nameElem != undefined && nameElem.className == "streamName streamName-" + streamdata.uuid) {
            nameElem.onclick();
            nameElem.onmouseout(); // Deselect the stream before removing it
        }
        var toRemove = self.find(".legend-" + streamdata.uuid);
        var selectElem = d3.select(toRemove).select('.axis-select').node();
        var oldAxis = selectElem[selectElem.selectedIndex].value;
        s3ui.changeAxis(self, streamdata, oldAxis, null);
        toRemove.parentNode.removeChild(toRemove);
        // we could delete self.idata.streamSettings[streamdata.uuid]; but I want to keep the settings around
        streamSettings[streamdata.uuid].active = false;
        for (var i = 0; i < self.idata.selectedStreamsBuffer.length; i++) {
            if (self.idata.selectedStreamsBuffer[i].uuid == streamdata.uuid) {
                self.idata.selectedStreamsBuffer.splice(i, 1);
                break;
            }
        }
        if (update) {
            s3ui.applySettings(self, false); // Make stream removal visible on the graph
        }
    }
}

/* Sets the message to be displayed for a certain importance; the message with
   the highest importanced is displayed. */
function setStreamMessage(self, uuid, message, importance) {
    var messages = self.idata.streamMessages[uuid];
    messages[0][importance] = message;
    var messageLoc;
    if (message == undefined) {
        if (importance == messages[1]) {
            while (messages[0][importance] == undefined) {
                importance--;
            }
            messages[1] = importance;
            messageLoc = self.find(".message-" + uuid);
            if (messageLoc != null) {
                messageLoc.innerHTML = messages[0][importance];
            }
        }
    } else if (importance >= messages[1]) {
        messages[1] = importance;
        messageLoc = self.find(".message-" + uuid)
        if (messageLoc != null) {
            messageLoc.innerHTML = message;
        }
    }
}

function updatePlotMessage(self) {
    var message = "";
    if (self.idata.changedTimes) {
        message = 'Click "Apply and Plot" to update the graph, using the selected the start and end times.';
    } else if (self.idata.addedStreams || self.idata.otherChange) {
        message = 'Click "Apply Settings" below to update the graph.';
    }
    self.find(".plotLoading").innerHTML = message;
}

function getSelectedTimezone(self) {
    var timezoneSelect = self.find(".timezoneSelect");
    var dst = (self.find(".dstButton").getAttribute("aria-pressed") == "true");
    var selection = timezoneSelect[timezoneSelect.selectedIndex].value;
    if (selection == "OTHER") {
        return [self.find(".otherTimezone").value.trim(), dst];
    } else {
        return [selection, dst];
    }
}

function createPlotDownload(self) {
    var chartElem = self.find(".chart");
    var chartData = chartElem.innerHTML.replace(/[\d.]+em/g, function (match) {
            return (parseFloat(match.slice(0, match.length - 2)) * 16) + "px";
        });
    var chartData = chartData.replace(">\n</tspan>", " font-color=\"white\"></tspan>"); // So it renders correctly in Inkview
    var graphStyle = self.find(".plotStyles").innerHTML;
    var xmlData = '<svg xmlns="http://www.w3.org/2000/svg" width="' + chartElem.getAttribute("width") + '" height="' + chartElem.getAttribute("height") + '" font-family="serif" font-size="16px">'
        + '<defs><style type="text/css"><![CDATA[' + graphStyle + ']]></style></defs>' + chartData + '</svg>';
    var downloadAnchor = document.createElement("a");
    downloadAnchor.innerHTML = "Download Image (created " + (new Date()).toLocaleString() + ", local time)";
    downloadAnchor.setAttribute("href", 'data:application/octet-stream;charset=utf-8,' + encodeURIComponent(xmlData));
    downloadAnchor.setAttribute("download", "graph.svg");
    var linkLocation = self.find(".download-graph");
    linkLocation.innerHTML = ""; // Clear what was there before...
    linkLocation.insertBefore(downloadAnchor, null); // ... and replace it with this download link
}

function createPermalink(self, return_raw_document) {
    if (self.idata.oldXScale == undefined) {
        return;
    }
    var coerce_stream;
    if (self.find(".includeMetadata").checked) {
        coerce_stream = function (stream) { return JSON.parse(JSON.stringify(stream)); };
    } else {
        coerce_stream = function (stream) { return stream.uuid; };
    }
    var domain = self.idata.oldXScale.domain();
    var streams = [];
    var permalink = {
            streams: self.idata.selectedStreams.map(function (d) { return { stream: coerce_stream(d), color: self.idata.streamSettings[d.uuid].color, selected: self.idata.showingDensity == d.uuid }; }),
            resetStart: Number(self.idata.oldStartDate.toString() + '000000'),
            resetEnd: Number(self.idata.oldEndDate.toString() + '000000'),
            tz: self.idata.oldTimezone,
            dst: self.idata.oldDST,
            start: Number((domain[0].getTime() - self.idata.offset).toString() + '000000'),
            end: Number((domain[1].getTime() - self.idata.offset).toString() + '000000'),
            autoupdate: self.idata.automaticAxisUpdate,
            axes: $.map(self.idata.yAxes, function (d) {
                    return {
                            axisname: d.truename,
                            streams: $.map(d.streams, function (s) { return s.uuid; }),
                            scale: d.manualscale,
                            rightside: d.right
                        };
                })
        };
    if (self.idata.vertCursor1) {
        permalink.vertCursor1 = self.idata.vertCursor1.coord / self.idata.WIDTH;
    }
    if (self.idata.vertCursor2) {
        permalink.vertCursor2 = self.idata.vertCursor2.coord / self.idata.WIDTH;
    }
    if (self.idata.horizCursor1) {
        permalink.horizCursor1 = 1 - self.idata.horizCursor1.coord / self.idata.HEIGHT;
    }
    if (self.idata.horizCursor2) {
        permalink.horizCursor2 = 1 - self.idata.horizCursor2.coord / self.idata.HEIGHT;
    }
    if (return_raw_document) {
        return permalink;
    }
    Meteor.call("createPermalink", permalink, function (error, result) {
            if (error == undefined) {
                var id = result;
                var URL = self.idata.initPermalink + id;
                var anchor = document.createElement("a");
                anchor.innerHTML = URL;
                anchor.setAttribute("href", URL);
                anchor.setAttribute("target", "_blank");
                var permalocation = self.find(".permalink");
                permalocation.innerHTML = "";
                permalocation.insertBefore(anchor, null);
                self.idata.loadedPermalink = true;
            } else {
                console.log(error);
            }
        });
    return true;
}

function buildCSVMenu(self) {
    var settingsObj = {};
    var graphExport = self.find("div.graphExport");
    var streamsettings = graphExport.querySelector("div.csv-streams");
    $(streamsettings).empty();
    var streams = self.idata.selectedStreams.slice(); // In case the list changes in the meantime
    var update, groups;
    if (streams.length > 0) {
        update = d3.select(streamsettings)
          .selectAll("div")
          .data(streams);
        groups = update.enter()
          .append("div")
            .attr("class", "input-group");
        groups.append("span")
            .attr("class", "input-group-btn")
          .append("div")
            .attr("class", "btn btn-default active")
            .attr("data-toggle", "button")
            .html("Included")
            .each(function () {
                    this.onclick = function () {
                            var streamName = this.parentNode.nextSibling;
                            if (this.innerHTML == "Included") {
                                this.innerHTML = "Include Stream";
                                delete settingsObj[this.__data__.uuid];
                                streamName.value = s3ui.getFilepath(this.__data__);
                            } else {
                                this.innerHTML = "Included";
                                settingsObj[this.__data__.uuid] = streamName.value;
                            }
                            streamName.disabled = !streamName.disabled;
                        };
                });
        groups.append("input")
            .attr("type", "text")
            .attr("class", "form-control")
            .property("value", function (d) { return s3ui.getFilepath(d); })
            .each(function () {
                    this.onchange = function () {
                            settingsObj[this.__data__.uuid] = this.value;
                        };
                    this.onchange();
                });
        update.exit().remove();
    } else {
        streamsettings.innerHTML = "You must plot streams in your desired time range before you can generate a CSV file.";
    }
    
    var pwselector = graphExport.querySelector(".pointwidth-selector");
    var domain = self.idata.oldXScale;
    var submitButton = graphExport.querySelector("div.csv-button");
    var $submitButton = $(submitButton);
    var textSpace;
    if (streams.length > 0 && domain != undefined) {
        domain = domain.domain();
        $(pwselector).css("display", "");
        pwselector.onchange = function () {
                var pw = Math.pow(2, 62 - this.value);
                var m1 = this.nextSibling.nextSibling;
                m1.innerHTML = "Point width: " + s3ui.nanosToUnit(pw) + " [exponent = " + (62 - this.value) + "]";
                var pps = Math.ceil(1000000 * (domain[1] - domain[0]) / pw);
                var statusString = "About " + pps + (pps == 1 ? " point per stream" : " points per stream");
                if (pps > 100000) {
                    $submitButton.addClass("disabled")
                    statusString += " <strong>(too many to download)</strong>"
                } else {
                    $submitButton.removeClass("disabled")
                }
                m1.nextSibling.nextSibling.innerHTML = statusString;
            };
        pwselector.value = 63 - self.idata.oldData[streams[0].uuid][2];
        pwselector.onchange();
        
        submitButton.onclick = function () {
                createCSVDownload(self, streams, settingsObj, domain, 62 - parseInt(pwselector.value), graphExport);
            };
    } else {
        $(pwselector).css("display", "none");
        textSpace = pwselector.nextSibling.nextSibling;
        textSpace.innerHTML = "You must plot streams in your desired time range before you can select a resolution.";
        textSpace.nextSibling.nextSibling.innerHTML = "";
        submitButton.onclick = function () { return false; };
    }
}

function createCSVDownload(self, streams, settingsObj, domain, pwe, graphExport) {
    streams = streams.filter(function (x) { return settingsObj.hasOwnProperty(x.uuid); }).map(function (x) { return x.uuid; });
    var dataJSON = {
            "UUIDS": streams,
            "Labels": streams.map(function (x) { return settingsObj[x]; }),
            "StartTime": domain[0] - self.idata.offset,
            "EndTime": domain[1] - self.idata.offset,
            "UnitOfTime": "ms",
            "PointWidth": pwe
        };
    var csvform = graphExport.querySelector(".csv-form");
    csvform.querySelector(".csv-form-data").value = JSON.stringify(dataJSON);
    csvform.submit();
}

s3ui.init_frontend = init_frontend;
s3ui.toggleLegend = toggleLegend;
s3ui.setStreamMessage = setStreamMessage;
s3ui.updatePlotMessage = updatePlotMessage;
s3ui.getSelectedTimezone = getSelectedTimezone;
s3ui.createPlotDownload = createPlotDownload;
s3ui.createPermalink = createPermalink;
s3ui.buildCSVMenu = buildCSVMenu;
