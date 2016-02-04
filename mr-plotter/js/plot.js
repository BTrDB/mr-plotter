// Stores state of graph and contains functions to manipulate it

function init_plot(self) {
    self.idata.initialized = false;
    
    // For the permalink
    self.idata.initzoom = 1;
    self.idata.inittrans = 0;

    // Margin size (not constant)
    self.idata.normalbmargin = 70;
    self.idata.cursorbmargin = 180;
    self.idata.margin = {left: 100, right: 100, top: 70, bottom: self.idata.normalbmargin};
    
    // Height of the chart area (constant)
    self.idata.HEIGHT = 300;
    
    // Width of the chart and chart area (WIDTH is set automatically by updateSize)
    self.idata.TARGETWIDTH = undefined;
    self.idata.WIDTH = undefined;
    self.idata.widthmin = 450;

    // Selection of the element to display progress
    self.idata.$loadingElem = self.$('.plotLoading');

    // Parameters of the last update
    self.idata.oldStartDate = undefined;
    self.idata.oldEndDate = undefined;
    self.idata.oldTimezone = undefined;
    self.idata.oldDST = undefined;
    self.idata.oldData = {};
    self.idata.oldXScale = undefined;
    self.idata.oldXAxis = undefined;
    self.idata.oldYScales = undefined;
    self.idata.oldYAxisArray = undefined;
    self.idata.oldAxisData = undefined;
    self.idata.offset = undefined;
    self.idata.oldDomain = undefined;
    
    self.idata.scriptsize = "0.75em";

    // Keeps track of whether the graph is drawn on the screen
    self.idata.onscreen = false;

    self.idata.selectedStreams = []; // The streams that are being displayed on the graph
    self.idata.drawRequestID = -1; // The ID of a request for "repaintZoomNewData"; if a later request is made and processed before an earlier one is processed, the earlier one is not processed

    // The uuid of the relevant stream if a data density plot is being shown, undefined otherwise
    self.idata.showingDensity = undefined;
    // Keeps track of whether the previous draw of the data density plot could be completed
    self.idata.drawnBefore = true;

    // The HTML elements showing the title of the x-axis, and the start and end dates
    self.idata.xTitle = undefined;
    self.idata.xStart = undefined;
    self.idata.xEnd = undefined;
    
    self.idata.zoom = d3.behavior.zoom()
        .on("zoomstart", function () { repaintZoomNewData(self, function () {}, true); })
        .on("zoom", function () { repaintZoom(self); })
        .on("zoomend", function () { repaintZoomNewData(self); })
        .size([self.idata.WIDTH, self.idata.HEIGHT]);
        
    self.idata.testElem = undefined;
    
    self.idata.cursorDataElems = {};
    self.idata.cursorDataElems.x1 = undefined;
    self.idata.cursorDataElems.x2 = undefined;
    self.idata.cursorDataElems.deltax = undefined;
    self.idata.cursorDataElems.freqx = undefined;
    self.idata.cursorDataElems.fx1 = undefined;
    self.idata.cursorDataElems.fx2 = undefined;
    self.idata.cursorDataElems.y1 = undefined;
    self.idata.cursorDataElems.y2 = undefined;
    self.idata.cursorDataElems.deltay = undefined;
    
    self.idata.$background = undefined;
    self.idata.cursorgroup = undefined;
    
    // The minimum width that an axis takes up is 100 pixels
}

// Behavior for zooming and scrolling
function repaintZoom(self) {
    d3.select(self.find("g.x-axis")).call(self.idata.oldXAxis);
    drawStreams(self, self.idata.oldData, self.idata.selectedStreams, self.idata.streamSettings, self.idata.oldXScale, self.idata.oldYScales, self.idata.oldYAxisArray, self.idata.oldAxisData, self.idata.$loadingElem, true);
}

// In these functions, I abbreviate point self.idata.WIDTH exponent with pwe

function cacheData(self, uuid, drawID, pwe, startTime, endTime) {
    var sideCache = endTime - startTime;
    if (drawID != self.idata.drawRequestID) {
        return;
    }
    s3ui.ensureData(self, uuid, pwe, startTime - sideCache, startTime,
        function () {
            if (drawID != self.idata.drawRequestID) {
                return;
            }
            s3ui.ensureData(self, uuid, pwe, endTime, endTime + sideCache,
            function () {
                if (drawID != self.idata.drawRequestID || pwe == 0) {
                    return;
                }
                s3ui.ensureData(self, uuid, pwe - 1, startTime - sideCache, endTime + sideCache,
                function () {
                    if (drawID != self.idata.drawRequestID || pwe == 1) {
                        return;
                    }
                    s3ui.ensureData(self, uuid, pwe + 1, startTime - sideCache, endTime + sideCache,
                    function () {
                        if (drawID != self.idata.drawRequestID) {
                            return;
                        }
                        s3ui.ensureData(self, uuid, pwe - 2, startTime - sideCache, endTime + sideCache, function () { s3ui.setStreamMessage(self, uuid, undefined, 1); }, true);
                    }, true);
                }, true);
            }, true);
        }, true);
}

function repaintZoomNewData(self, callback, stopCache, widthEstimate) {
    if (callback == undefined) {
        callback = function () { repaintZoom(self); };
    }
    var selectedStreams = self.idata.selectedStreams;
    var domain = self.idata.oldXScale.domain();
    self.idata.xStart.innerHTML = self.idata.labelFormatter.format(domain[0]);
    self.idata.xEnd.innerHTML = self.idata.labelFormatter.format(domain[1]);
    s3ui.updateVertCursorStats(self);
    var numResponses = 0;
    function makeDataCallback(stream, startTime, endTime) {
        return function (data, low, high) {
            if (thisID != self.idata.drawRequestID) { // another request has been made
                return;
            }
            if (!self.idata.pollingBrackets && s3ui.shouldPollBrackets(self, stream.uuid, domain)) {
                s3ui.startPollingBrackets(self);
            }
            s3ui.limitMemory(self, selectedStreams, self.idata.oldOffsets, domain[0], domain[1], 300000 * selectedStreams.length, 150000 * selectedStreams.length);
            if (data != undefined) {
                self.idata.oldData[stream.uuid] = [stream, data, pwe, low, high];
            }
            numResponses++;
            s3ui.setStreamMessage(self, stream.uuid, undefined, 5);
            if (!stopCache) {
                s3ui.setStreamMessage(self, stream.uuid, "Caching data...", 1);
                setTimeout(function () { cacheData(self, stream.uuid, thisID, pwe, startTime, endTime); }, 0); // do it asynchronously
            }
            if (numResponses == selectedStreams.length) {
                s3ui.updateVertCursorStats(self);
                callback();
            }
        };
    }
    if (widthEstimate == undefined) {
        widthEstimate = self.idata.WIDTH;
    }
    var pwe = s3ui.getPWExponent((domain[1] - domain[0]) / widthEstimate);
    var thisID = ++self.idata.drawRequestID;
    if (self.idata.drawRequestID > 8000000) {
        self.idata.drawRequestID = -1;
    }
    for (var i = 0; i < selectedStreams.length; i++) {
        s3ui.setStreamMessage(self, selectedStreams[i].uuid, "Fetching data...", 5);
        s3ui.ensureData(self, selectedStreams[i].uuid, pwe, domain[0] - self.idata.offset, domain[1] - self.idata.offset, makeDataCallback(selectedStreams[i], domain[0] - self.idata.offset, domain[1] - self.idata.offset));
    }
    if (selectedStreams.length == 0) {
        callback();
    }
}

function initPlot(self) {
    var chart = d3.select(self.find("svg.chart"));
    $(chart.node()).empty(); // Remove directions from inside the chart
    self.idata.testElem = chart.append("g")
        .attr("class", "tick")
      .append("text")
        .style("visibility", "none")
        .node();
    chart.attr("width", self.idata.margin.left + self.idata.WIDTH + self.idata.margin.right)
        .attr("height", self.idata.margin.top + self.idata.HEIGHT + self.idata.margin.bottom)
      .append("rect")
        .attr("class", "background-rect")
        .attr("fill", "white")
        .attr("width", self.idata.margin.left + self.idata.WIDTH + self.idata.margin.right)
        .attr("height", self.idata.margin.top + self.idata.HEIGHT + self.idata.margin.bottom)
    var chartarea = chart.append("g")
        .attr("class", "chartarea")
        .attr("width", self.idata.WIDTH)
        .attr("height", self.idata.HEIGHT)
        .attr("transform", "translate(" + self.idata.margin.left + ", " + self.idata.margin.top + ")");
    var yaxiscover = chart.append("g")
        .attr("class", "y-axis-cover axiscover");
    yaxiscover.append("rect")
        .attr("width", self.idata.margin.left)
        .attr("height", self.idata.margin.top + self.idata.HEIGHT + self.idata.margin.bottom)
        .attr("class", "y-axis-background-left")
        .attr("fill", "white");
    yaxiscover.append("rect")
        .attr("width", self.idata.margin.right)
        .attr("height", self.idata.margin.top + self.idata.HEIGHT + self.idata.margin.bottom)
        .attr("transform", "translate(" + (self.idata.margin.left + self.idata.WIDTH) + ", 0)")
        .attr("class", "y-axis-background-right")
        .attr("fill", "white");
    var xaxiscover = chart.append("g")
        .attr("class", "x-axis-cover")
        .attr("transform", "translate(" + self.idata.margin.left + ", " + (self.idata.margin.top + self.idata.HEIGHT) + ")");
    xaxiscover.append("rect")
        .attr("width", self.idata.WIDTH + 2) // Move 1 to the left and increase width by 2 to cover boundaries when zooming
        .attr("height", self.idata.margin.bottom)
        .attr("transform", "translate(-1, 0)")
        .attr("class", "x-axis-background")
        .attr("fill", "white");
    self.idata.xTitle = xaxiscover.append("text")
        .attr("class", "xtitle title")
        .attr("text-anchor", "middle")
        .attr("x", self.idata.WIDTH / 2)
        .attr("y", 53)
        .html("Time")
      .node();
    self.idata.xStart = xaxiscover.append("text")
        .attr("text-anchor", "middle")
        .attr("class", "label")
        .attr("x", 0)
        .attr("y", 35)
      .node();
    self.idata.xEnd = xaxiscover.append("text")
        .attr("text-anchor", "middle")
        .attr("class", "label")
        .attr("x", self.idata.WIDTH)
        .attr("y", 35)
      .node();
    var scriptsize = self.idata.scriptsize;
    var subscriptoffset = "4px";
    var superscriptoffset = "6px";
    var cursors = self.idata.cursorDataElems;
    var alignoffset = 70;
    var x1 = xaxiscover.append("text")
        .attr("text-anchor", "start")
        .attr("class", "cursorlabel")
        .attr("x", -alignoffset)
        .attr("y", 75)
    x1.append("tspan")
        .html("x");
    x1.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("1");
    x1.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" = ");
    cursors.x1 = [x1.node(), x1.append("tspan").node()];
    s3ui.hideEntry(cursors.x1);
    
    var x2 = xaxiscover.append("text")
        .attr("text-anchor", "end")
        .attr("class", "cursorlabel cursor-right-align")
        .attr("x", self.idata.WIDTH + alignoffset)
        .attr("y", 75)
    x2.append("tspan")
        .html("x");
    x2.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("2");
    x2.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" = ");
    cursors.x2 = [x2.node(), x2.append("tspan").node()];
    s3ui.hideEntry(cursors.x2);
    
    var deltax = xaxiscover.append("text")
        .attr("text-anchor", "start")
        .attr("class", "cursorlabel")
        .attr("x", -alignoffset)
        .attr("y", 95);
    deltax.append("tspan")
        .html("x");
    deltax.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("2");
    deltax.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" - x");
    deltax.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("1");
    deltax.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" = ");
    cursors.deltax = [deltax.node(), deltax.append("tspan").node()];
    deltax.append("tspan")
        .html(" ns");
    s3ui.hideEntry(cursors.deltax);
    
    var freqx = xaxiscover.append("text")
        .attr("text-anchor", "end")
        .attr("class", "cursorlabel cursor-right-align")
        .attr("x", self.idata.WIDTH + alignoffset)
        .attr("y", 95);
    freqx.append("tspan")
        .html("(x");
    freqx.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("2");
    freqx.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" - x");
    freqx.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("1");
    freqx.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(")");
    freqx.append("tspan")
        .attr("dy", "-" + superscriptoffset)
        .attr("font-size", scriptsize)
        .html("-1");
    freqx.append("tspan")
        .attr("dy", superscriptoffset)
        .html(" = ");
    cursors.freqx = [freqx.node(), freqx.append("tspan").node()];
    freqx.append("tspan")
        .html(" Hz");
    s3ui.hideEntry(cursors.freqx);
    
    var fx1 = xaxiscover.append("g");
    var fx11 = fx1.append("text")
        .attr("text-anchor", "start")
        .attr("class", "cursorlabel")
        .attr("x", -alignoffset)
        .attr("y", 115);
    fx11.append("tspan")
        .html("Left: (");
    var fx1top = fx11.append("tspan").node();
    var fx1exp = fx11.append("tspan").attr("dy", "-" + superscriptoffset).attr("font-size", scriptsize).node();
    fx11.append("tspan")
        .attr("dy", superscriptoffset)
        .html(" ns,");
    var fx12 = fx1.append("text")
      .attr("text-anchor", "start")
        .attr("class", "cursorlabel")
        .attr("x", -alignoffset)
        .attr("y", 130);
    fx12.append("tspan")
        .style("fill", "none")
        .html("Left: (");
    var fx1bottom = fx12.append("tspan")
        .node();
    fx12.append("tspan")
        .html(")");
    cursors.fx1 = [fx1.node(), fx1top, fx1exp, fx1bottom];
    s3ui.hideEntry(cursors.fx1);
    
    var fx2 = xaxiscover.append("g");
    var fx21 = fx2.append("text")
        .attr("text-anchor", "end")
        .attr("class", "cursorlabel cursor-right-align")
        .attr("x", self.idata.WIDTH + alignoffset)
        .attr("y", 115);
    fx21.append("tspan")
        .html("Right: (");
    var fx2top = fx21.append("tspan").node();
    var fx2exp = fx21.append("tspan").attr("dy", "-" + superscriptoffset).attr("font-size", scriptsize).node();
    fx21.append("tspan")
        .attr("dy", superscriptoffset)
        .html(" ns,");
    var fx22 = fx2.append("text")
        .attr("text-anchor", "end")
        .attr("class", "cursorlabel cursor-right-align")
        .attr("x", self.idata.WIDTH + alignoffset)
        .attr("y", 130);
    var fx2bottom = fx22.append("tspan")
        .node();
    fx22.append("tspan")
        .html(")");
    cursors.fx2 = [fx2.node(), fx2top, fx2exp, fx2bottom];
    s3ui.hideEntry(cursors.fx2);
    
    var y1 = xaxiscover.append("text")
        .attr("text-anchor", "start")
        .attr("class", "cursorlabel")
        .attr("x", -alignoffset)
        .attr("y", 150);
    y1.append("tspan")
        .html("y");
    y1.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("1");
    y1.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" = ");
    cursors.y1 = [y1.node(), y1.append("tspan").node()];
    s3ui.hideEntry(cursors.y1);
    
    var y2 = xaxiscover.append("text")
        .attr("text-anchor", "end")
        .attr("class", "cursorlabel cursor-right-align")
        .attr("x", self.idata.WIDTH + alignoffset)
        .attr("y", 150);
    y2.append("tspan")
        .html("y");
    y2.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("2");
    y2.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" = ");
    cursors.y2 = [y2.node(), y2.append("tspan").node()];
    s3ui.hideEntry(cursors.y2);
        
    var deltay = xaxiscover.append("text")
        .attr("text-anchor", "middle")
        .attr("class", "cursorlabel")
        .attr("x", self.idata.WIDTH / 2)
        .attr("y", 170);
    deltay.append("tspan")
        .html("y");
    deltay.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("2");
    deltay.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" - ");
    deltay.append("tspan")
        .html("y");
    deltay.append("tspan")
        .attr("dy", subscriptoffset)
        .attr("font-size", scriptsize)
        .html("1");
    deltay.append("tspan")
        .attr("dy", "-" + subscriptoffset)
        .html(" = ");
    cursors.deltay = [deltay.node(), deltay.append("tspan").node()];
    s3ui.hideEntry(cursors.deltay);
        
    var datadensitycover = chart.append("g")
        .attr("class", "data-density-cover")
        .attr("transform", "translate(" + self.idata.margin.left + ", 0)");
    datadensitycover.append("rect") // Move 1 to the left and increase width by 2 to cover boundaries when zooming
        .attr("width", self.idata.WIDTH + 2)
        .attr("height", self.idata.margin.top)
        .attr("transform", "translate(-1, 0)")
        .attr("class", "data-density-background")
        .attr("fill", "white");
    xaxiscover.append("g")
        .attr("class", "x-axis axis");
    var yaxes = chart.append("g")
        .attr("transform", "translate(0, " + self.idata.margin.top + ")")
        .attr("class", "y-axes")
    yaxes.append("g")
        .attr("class", "y-axes-left");
    yaxes.append("g")
        .attr("transform", "translate(" + (self.idata.margin.left + self.idata.WIDTH) + ", 0)")
        .attr("class", "y-axes-right");
    datadensitycover.append("g")
        .attr("transform", "translate(0, 10)")
        .attr("class", "data-density-plot")
      .append("g")
        .attr("class", "data-density-axis");
    var plotclickscreen = chart.append("rect") // To sense mouse click/drag
        .attr("width", self.idata.WIDTH)
        .attr("height", self.idata.HEIGHT)
        .attr("transform", "translate(" + self.idata.margin.left + ", " + self.idata.margin.top + ")")
        .attr("class", "plotclickscreen clickscreen unclickedchart")
      .node();
    plotclickscreen.onmousedown = function () {
            $(this).attr('class', 'plotclickscreen clickscreen clickedchart');
        };
    plotclickscreen.onmouseup = function () {
            $(this).attr('class', 'plotclickscreen clickscreen unclickedchart');
        };
    var bottomcursorselect = chart.append("rect")
        .attr("width", self.idata.WIDTH)
        .attr("height", self.idata.margin.bottom)
        .attr("transform", "translate(" + self.idata.margin.left + ", " + (self.idata.margin.top + self.idata.HEIGHT) + ")")
        .attr("class", "clickscreen bottomcursorselect")
      .node();
    $(bottomcursorselect).mousedown(function (event) {
            var newCursor = self.imethods.createVerticalCursor(event.pageX - (self.idata.margin.left + $(chart.node()).offset().left));
            if (newCursor) {
                newCursor.select(event.pageX);
            }
        });
    var leftcursorselect = chart.append("rect")
        .attr("width", self.idata.margin.left)
        .attr("height", self.idata.HEIGHT)
        .attr("class", "clickscreen leftcursorselect")
        .attr("transform", "translate(0, " + self.idata.margin.top + ")")
      .node();
    var createHorizCursor = function (event) {
        var newCursor = self.imethods.createHorizontalCursor(event.pageY - (self.idata.margin.top + $(chart.node()).offset().top));
        if (newCursor) {
            newCursor.select(event.pageY);
        }
    };
    $(leftcursorselect).mousedown(createHorizCursor);
    var rightcursorselect = chart.append("rect")
        .attr("width", self.idata.margin.right)
        .attr("height", self.idata.HEIGHT)
        .attr("class", "clickscreen rightcursorselect")
        .attr("transform", "translate(" + (self.idata.margin.left + self.idata.WIDTH) + ", " + self.idata.margin.top + ")")
      .node();
    $(rightcursorselect).mousedown(createHorizCursor);
    self.idata.cursorgroup = chart.append("g")
        .attr("transform", "translate(" + self.idata.margin.left + ", " + self.idata.margin.top + ")")
        .attr("class", "cursorgroup");
    self.idata.$background = $("svg.chart > .clickscreen, svg.chart .data-density-background, svg.chart .y-axis-background-left, svg.chart .y-axis-background-right");
    self.idata.$loadingElem = $(self.find('.plotLoading'));
    self.idata.initialized = true;
}

/* Updates the size of the chart based on changes to the margins. The width will
   be changed to best match self.idata.TARGETWIDTH. */
function updateSize(self, redraw) {
    var oldwidth = self.idata.WIDTH;
    var margin = self.idata.margin;
    
    self.idata.WIDTH = Math.max(self.idata.widthmin, self.idata.TARGETWIDTH - margin.left - margin.right);
    var WIDTH = self.idata.WIDTH;
    var HEIGHT = self.idata.HEIGHT;
    self.idata.zoom.size([WIDTH, HEIGHT]);
    self.$("svg.chart, svg.chart rect.background-rect").attr({
            width: margin.left + WIDTH + margin.right,
            height: margin.top + HEIGHT + margin.bottom
        });
    self.$("svg.chart g.chartarea, svg.chart rect.plotclickscreen, svg.chart g.cursorgroup").attr({
            transform: "translate(" + margin.left + ", " + margin.top + ")",
            width: WIDTH
        });
    self.$("svg.chart g.x-axis-cover, svg.chart rect.bottomcursorselect").attr("transform", "translate(" + margin.left + ", " + (margin.top + HEIGHT) + ")");
    self.$("svg.chart g.data-density-cover").attr("transform", "translate(" + margin.left + ", 0)");
    self.$("rect.x-axis-background").attr({
            height: margin.bottom,
            width: WIDTH + 2
        });
    self.$("rect.y-axis-background-left").attr({
            width: margin.left,
            height: margin.top + HEIGHT + margin.bottom
        });
    self.$("rect.y-axis-background-right").attr({
            width: margin.right,
            height: margin.top + HEIGHT + margin.bottom,
            transform: "translate(" + (margin.left + WIDTH) + ", 0)"
        });
    $(self.find("rect.leftcursorselect")).attr({
            width: margin.left,
            height: HEIGHT,
            transform: "translate(0, " + margin.top + ")"
        });
    $(self.find("rect.rightcursorselect")).attr({
            width: margin.right,
            height: HEIGHT,
            transform: "translate(" + (margin.left + WIDTH) + ", " + margin.top + ")"
        });
    $(self.find("rect.bottomcursorselect")).attr({
            width: WIDTH,
            height: margin.bottom
        });
    self.$("rect.data-density-background").attr({
            width: WIDTH + 2,
            height: margin.top
        });
    self.$("g.y-axes").attr("transform", "translate(0, " + margin.top + ")");
    self.$("g.y-axes-right").attr("transform", "translate(" + (margin.left + WIDTH) + ", 0)");
    if (oldwidth == WIDTH || !self.idata.initialized) {
        return;
    }
    var zoom = self.idata.zoom;
    zoom.size([WIDTH, HEIGHT]);
    self.idata.xTitle.setAttribute("x", WIDTH / 2);
    self.idata.xEnd.setAttribute("x", WIDTH);
    self.$("svg.chart text.cursor-right-align").attr("x", WIDTH + 80);
    self.idata.cursorDataElems.deltay[0].setAttribute("x", WIDTH / 2);
    // Deal with cursor display
    var oldXScale = self.idata.oldXScale;
    if (self.idata.oldXScale != undefined) {
        var scale = zoom.scale();
        var translate = zoom.translate()[0];
        oldXScale.domain(self.idata.oldDomain);
        oldXScale.range([0, WIDTH]);
        zoom.x(oldXScale).scale(scale).translate([translate / oldwidth * WIDTH, 0]);
        self.idata.oldXAxis(d3.select(self.find("g.x-axis")));
        // I could do it more efficiently if I just mutated the object's attributes directly rather than creating
        // methods, but that would be much less readable (enough to make up for the slowness of checking this.vertical
        // twice unnecessarily)
        if (self.idata.vertCursor1 != undefined) {
            self.idata.vertCursor1.updateCoordinate(self.idata.vertCursor1.coord / oldwidth * WIDTH);
        }
        if (self.idata.vertCursor2 != undefined) {
            self.idata.vertCursor2.updateCoordinate(self.idata.vertCursor2.coord / oldwidth * WIDTH);
        }
        if (self.idata.horizCursor1 != undefined) {
            self.idata.horizCursor1.updateLength(WIDTH);
        }
        if (self.idata.horizCursor2 != undefined) {
            self.idata.horizCursor2.updateLength(WIDTH);
        }
        // The height can't change, so we're done. If the height could also change, I'd also have to update
        // the heights of the vertical cursors and the positions of the horizontal cursors.
    }
    if (redraw) {
        setTimeout(function () { repaintZoomNewData(self); }, 50);
    }
}

function updatePlot(self) {
    if (!self.idata.automaticAxisUpdate) {
        self.idata.selectedStreams = self.idata.selectedStreamsBuffer.slice();
    }
    if (!self.idata.initialized) {
        initPlot(self);
    }
    drawPlot(self);
}

function applySettings(self, loadData, overrideAutomaticAxisUpdate) {
    if (self.idata.onscreen) {
        if (!(self.idata.automaticAxisUpdate || overrideAutomaticAxisUpdate)) {
            self.idata.otherChange = true;
            s3ui.updatePlotMessage(self);
        } else {
            if (loadData) {
                repaintZoomNewData(self, function () {
                        drawYAxes(self, self.idata.oldData, self.idata.selectedStreams, self.idata.streamSettings, self.idata.oldStartDate, self.idata.oldEndDate, self.idata.oldXScale, self.idata.$loadingElem);
                    });
            }
            drawYAxes(self, self.idata.oldData, self.idata.selectedStreams, self.idata.streamSettings, self.idata.oldStartDate, self.idata.oldEndDate, self.idata.oldXScale, self.idata.$loadingElem);
        }
    }
}

function drawPlot(self) {
    // Get the time range we are going to plot
    var $loadingElem = self.idata.$loadingElem;
    $loadingElem.html("Verifying date range...");
    var startText = self.find(".startdate").value;
    var endText = self.find(".enddate").value;
    if (startText == "") {
        $loadingElem.html("Error: Start date is not selected.");
        return;
    } else if (endText == "") {
        $loadingElem.html("Error: End date is not selected.");
        return;
    }
    var selectedTimezone = s3ui.getSelectedTimezone(self);
    var dst = selectedTimezone[1];
    selectedTimezone = selectedTimezone[0];
    self.idata.oldTimezone = selectedTimezone;
    self.idata.oldDST = dst;
    
    var naiveStartDateObj = self.idata.dateConverter.parse(startText);
    var naiveEndDateObj = self.idata.dateConverter.parse(endText);
    try {
        var tz = s3ui.getTimezoneOffsetMinutes(selectedTimezone, dst, true)
        self.idata.offset = tz[0] * -60000; // what to add to UTC to get to selected time zone
        self.idata.xTitle.innerHTML = "Time [" + selectedTimezone + " (" + tz[1] + ")]";
        var startDateObj = new timezoneJS.Date(naiveStartDateObj.getFullYear(), naiveStartDateObj.getMonth(), naiveStartDateObj.getDate(), naiveStartDateObj.getHours(), naiveStartDateObj.getMinutes(), naiveStartDateObj.getSeconds(), 'UTC');
        var endDateObj = new timezoneJS.Date(naiveEndDateObj.getFullYear(), naiveEndDateObj.getMonth(), naiveEndDateObj.getDate(), naiveEndDateObj.getHours(), naiveEndDateObj.getMinutes(), naiveEndDateObj.getSeconds(), 'UTC');
        // startDateObj.getTime() and endDateObj.getTime() are in selected time zone
        var startDate = startDateObj.getTime() - self.idata.offset;
        var endDate = endDateObj.getTime() - self.idata.offset;
        // startDate and endDate are in UTC
    } catch (err) {
        $loadingElem.html(err);
        return;
    }
    if (startDate >= endDate) {
        $loadingElem.html("Error: Selected date range is invalid.");
        return;
    }
    
    /* Used for optimization; GET request is not sent if same time range and streams are used. */
    var sameTimeRange = ((startDate == self.idata.oldStartDate) && (endDate == self.idata.oldEndDate));
    
    // Verify that streams have been selected
    $loadingElem.html("Verifying stream selection...");
    var numstreams = self.idata.selectedStreams.length;
    if (numstreams == 0) {
        $loadingElem.html("Error: No streams are selected.");
        return;
    }
    
    self.idata.oldDomain = [startDate + self.idata.offset, endDate + self.idata.offset];
    // Create the xScale and axis if we need to
    var xScale, xAxis;
    if (!sameTimeRange) {
        xScale = d3.time.scale.utc() // I'm telling d3 it's in UTC time, but in reality I'm going to add an offset to everything so it actually displays the selected time zone
            .domain([startDate + self.idata.offset, endDate + self.idata.offset])
            .range([0, self.idata.WIDTH]);
        xAxis = d3.svg.axis().scale(xScale).orient("bottom").ticks(5);
        self.idata.oldStartDate = startDate;
        self.idata.oldEndDate = endDate;
        self.idata.oldXScale = xScale;
        self.idata.oldXAxis = xAxis;
        self.idata.zoom.scaleExtent([(endDate - startDate) / 315360000000000, endDate - startDate]); // So we don't zoom in past 1 ms, or zoom out past 10000 years
    } else {
        xScale = self.idata.oldXScale;
        xAxis = self.idata.oldXAxis;
    }
    
    $loadingElem.html("Fetching data...");
    
    self.idata.zoom.x(xScale);
    self.idata.zoom.scale(self.idata.initzoom).translate([self.idata.inittrans, 0]);
    self.idata.initzoom = 1;
    self.idata.inittrans = 0;
    
    var leftCount = -1;
    var rightCount = -1;
    for (var i = 0; i < self.idata.yAxes.length; i++) {
        if (self.idata.yAxes[i].right === false) {
            leftCount++;
        } else if (self.idata.yAxes[i].right === true) {
            rightCount++;
        }
    }
    leftCount = Math.max(0, leftCount);
    rightCount = Math.max(0, rightCount);
    var estimatedWidthAfterAxes = Math.max(self.idata.widthmin, self.idata.WIDTH - 100 * (leftCount + rightCount));
    // an upper bound on the width we'll have after adding axes, so our estimated point width exponent is less likely to be too high
    
    // Get the data for the streams
    repaintZoomNewData(self, function () {
            if (!sameTimeRange) {
                d3.select(self.find("g.x-axis"))
                    .call(xAxis);
            }
            $loadingElem.html("Drawing graph...");
            // Set a timeout so the new message (Drawing graph...) actually shows
            setTimeout(function () { d3.select(".plotclickscreen").call(self.idata.zoom); drawYAxes(self, self.idata.oldData, self.idata.selectedStreams, self.idata.streamSettings, self.idata.oldStartDate, self.idata.oldEndDate, self.idata.oldXScale, $loadingElem); }, 50);
        }, false, estimatedWidthAfterAxes);
}

function drawYAxes(self, data, streams, streamSettings, startDate, endDate, xScale, $loadingElem) {
    otherChange = false;
    
    var yAxes = self.idata.yAxes;
    var i, j, k;
        
    // Find the minimum and maximum value in each stream to properly scale the axes
    var axisData = {}; // Maps axis ID to a 2-element array containing the minimum and maximum; later on a third element is added containing the y-Axis scale
    var toDraw = [];
    var numstreams;
    var streamdata;
    var totalmin;
    var totalmax;
    var datapointmin, datapointmax;
    var axis;
    var startIndex, endIndex;
    var domain = xScale.domain();
    var startTime = domain[0] - self.idata.offset;
    var endTime = domain[1] - self.idata.offset;
    for (i = 0; i < yAxes.length; i++) {
        axis = yAxes[i];
        numstreams = axis.streams.length;
        if (numstreams > 0) {
            toDraw.push(axis);
        }
        if (axis.newaxis && (axis.leftBox.value != "" || axis.rightBox.value != "")) { // Check if the user gave this axis an initial scale
            axis.newaxis = false;
            axis.autoscale = false;
        }
        if (!axis.autoscale && (axis.manualscale[1] > axis.manualscale[0])) {
            axisData[axis.axisid] = [NaN, NaN, undefined, true]; // so we know that we're using a manual scale for this axis
            continue;
        }
        totalmin = undefined;
        totalmax = undefined;
        for (j = 0; j < numstreams; j++) {
            if (!data.hasOwnProperty(axis.streams[j].uuid)) {
                continue;
            }
            streamdata = data[axis.streams[j].uuid][1];
            startIndex = s3ui.binSearchCmp(streamdata, [startTime, 0], s3ui.cmpTimes);
            if (startIndex < streamdata.length && s3ui.cmpTimes(streamdata[startIndex], [startTime, 0]) < 0) {
                startIndex++; // make sure we only look at data in the specified range
            }
            endIndex = s3ui.binSearchCmp(streamdata, [endTime, 0], s3ui.cmpTimes);
            if (endIndex < streamdata.length && s3ui.cmpTimes(streamdata[endIndex], endTime)) {
                endIndex--; // make sure we only look at data in the specified range
            }
            for (k = startIndex; k < endIndex; k++) {
                datapointmin = streamdata[k][2];
                datapointmax = streamdata[k][4];
                if (!(totalmin <= datapointmin)) {
                    totalmin = datapointmin;
                }
                if (!(totalmax >= datapointmax)) {
                    totalmax = datapointmax;
                }
            }
        }
        if (totalmin != undefined) {
            if (totalmin == totalmax) { // Choose a range so the axis can show something meaningful
                totalmin--;
                totalmax++;
            }
            axisData[axis.axisid] = [totalmin, totalmax, undefined, true];
        } else {
            axisData[axis.axisid] = [-1, 1, undefined, false];
        }
    }
    
    // Generate names for new axes if not overridden
    var axisnameelem;
    for (i = 0; i < toDraw.length; i++) {
        if (toDraw[i].newaxis && toDraw[i].axisname === toDraw[i].axisid) {
            axisnameelem = self.find(".axis-" + toDraw[i].axisid + " > td > .axisname");
            axisnameelem.value = s3ui.getUnitString(toDraw[i].units);
            axisnameelem.onchange();
        }
    }
    
    self.idata.oldAxisData = axisData;    
    
    numstreams = streams.length;
    
    var yScales = $.map(toDraw, function (elem) {
            var scale;
            if (isNaN(axisData[elem.axisid][0])) { // manual scale
                scale = d3.scale.linear()
                    .domain([elem.manualscale[0], elem.manualscale[1]])
                    .range([self.idata.HEIGHT, 0]);
                elem.newaxis = false;
            } else { // auto scale
                scale = d3.scale.linear()
                    .domain([axisData[elem.axisid][0], axisData[elem.axisid][1]])
                    .range([self.idata.HEIGHT, 0])
                    .nice();
                var domain = scale.domain();
                if (elem.autoscale) { // if this is the result of an AUTOSCALE rather than bad input...
                    if (axisData[elem.axisid][3]) { // only set the text in the axes if autoscale came up with something reasonable
                        elem.leftBox.value = domain[0];
                        elem.rightBox.value = domain[1];
                        elem.newaxis = false;
                    }
                    if (!elem.newaxis) {
                        elem.autoscale = false;
                    }
                }
                elem.manualscale[0] = domain[0];
                elem.manualscale[1] = domain[1];
            }
            axisData[elem.axisid][2] = scale;
            return scale;
        });
        
    self.idata.oldYScales = yScales;
    
    var yAxisArray = $.map(yScales, function (yScale) { return d3.svg.axis().scale(yScale).ticks(5); });
    
    var leftYAxes = [];
    var leftYObjs = [];
    var rightYAxes = [];
    var rightYObjs = [];
    for (i = 0; i < toDraw.length; i++) {
        if (toDraw[i].right === null) {
            continue;
        } else if (toDraw[i].right) {
            rightYAxes.push(yAxisArray[i]);
            rightYObjs.push(toDraw[i]);
        } else {
            leftYAxes.push(yAxisArray[i]);
            leftYObjs.push(toDraw[i]);
        }
    }
    
    self.idata.oldYAxisArray = yAxisArray;
    var leftMargins = leftYAxes.map(function (axis) { var scale = axis.scale(); return 65 + Math.max(35, Math.max.apply(this, scale.ticks().map(function (d) { self.idata.testElem.innerHTML = scale.tickFormat()(d); return self.idata.testElem.getComputedTextLength(); }))); });
    var rightMargins = rightYAxes.map(function (axis) { var scale = axis.scale(); return 65 + Math.max(35, Math.max.apply(this, scale.ticks().map(function (d) { self.idata.testElem.innerHTML = scale.tickFormat()(d); return self.idata.testElem.getComputedTextLength(); }))); });
    for (i = 1; i < leftMargins.length; i++) {
        leftMargins[i] += leftMargins[i - 1];
    }
    leftMargins.unshift(0);
    for (i = 1; i < rightMargins.length; i++) {
        rightMargins[i] += rightMargins[i - 1];
    }
    rightMargins.unshift(0);
    self.idata.margin.left = Math.max(100, leftMargins[leftMargins.length - 1]);
    self.idata.margin.right = Math.max(100, rightMargins[rightMargins.length - 1]);
    updateSize(self, false);
    
    // Draw the y-axes
    var update;
    update = d3.select(self.find("svg.chart g.y-axes"))
      .selectAll("g.y-axis-left")
      .data(leftYAxes);
    update.enter()
      .append("g");
    update
        .attr("transform", function (d, i) { return "translate(" + (self.idata.margin.left - leftMargins[i]) + ", 0)"; })
        .attr("class", function (d, i) { return "y-axis-left axis drawnAxis-" + leftYObjs[i].axisid; })
        .each(function (yAxis) { d3.select(this).call(yAxis.orient("left")); });
    update.exit().remove();
    
    update = d3.select(self.find("svg.chart g.y-axes-right"))
      .selectAll("g.y-axis-right")
      .data(rightYAxes);
    update.enter()
      .append("g");
    update
        .attr("transform", function (d, i) { return "translate(" + rightMargins[i] + ", 0)"; })
        .attr("class", function (d, i) { return "y-axis-right axis drawnAxis-" + rightYObjs[i].axisid; })
        .each(function (yAxis) { d3.select(this).call(yAxis.orient("right")); });
    update.exit().remove();
    
    // Draw the y-axis titles
    update = d3.select(self.find("svg.chart g.y-axes-left"))
      .selectAll("text.ytitle")
      .data(leftYObjs);
    update.enter()
      .append("text");
    update
        .attr("class", function (d) { return "ytitle title axistitle-" + d.axisid; })
        .attr("text-anchor", "middle")
        .attr("transform", (function () {
                var j = 0; // index of left axis
                return function (d) {
                    return "translate(" + (self.idata.margin.left - leftMargins[++j] + 40) + ", " + (self.idata.HEIGHT / 2) + ")rotate(-90)";
                };
             })())
        .html(function (d) { return d.axisname; });
    update.exit().remove();
    update = d3.select(self.find("svg.chart g.y-axes-right"))
      .selectAll("text.ytitle")
      .data(rightYObjs);
    update.enter()
      .append("text");
    update
        .attr("class", function (d) { return "ytitle title axistitle-" + d.axisid; })
        .attr("text-anchor", "middle")
        .attr("transform", (function () {
                var i = 0; // index of right axis
                return function (d) {
                    return "translate(" + (rightMargins[++i] - 40) + ", " + (self.idata.HEIGHT / 2) + ")rotate(90)";
                };
             })())
        .html(function (d) { return d.axisname; });
    update.exit().remove();
    
    s3ui.updateHorizCursorStats(self);
    
    for (var i = 0; i < toDraw.length; i++) {
        s3ui.applyDisplayColor(self, toDraw[i], streamSettings);
    }
    
    drawStreams(self, data, streams, streamSettings, xScale, yScales, yAxisArray, axisData, $loadingElem, false);
}

/* Render the graph on the screen. If DRAWFAST is set to true, the entire plot is not drawn (for the sake of speed); in
   paticular new streams are not added and old ones not removed (DRAWFAST tells it to optimize for scrolling).
*/
function drawStreams (self, data, streams, streamSettings, xScale, yScales, yAxisArray, axisData, $loadingElem, drawFast) {
    if (self.idata.loadedData) {
        self.find(".permalink").innerHTML = "";
    }
    if (!drawFast && (streams.length == 0 || yAxisArray.length == 0)) {
        if (streams.length == 0) {
            $loadingElem.html("Error: No streams are selected.");
        } else {
            $loadingElem.html("Error: All selected streams have no data.");
        }
        self.$("g.chartarea > g").remove();
        return;
    }
    if (yAxisArray == undefined) {
        return;
    }
    self.idata.onscreen = true;
    // Render the graph
    var update;
    var uuid;
    var dataArray = [];
    var yScale;
    var minval, mean, maxval;
    var subsetdata;
    var scaledX;
    var startIndex;
    var domain = xScale.domain();
    var startTime, endTime;
    var xPixel;
    var color;
    var mint, maxt;
    var outOfRange;
    var WIDTH = self.idata.WIDTH;
    var HEIGHT = self.idata.HEIGHT;
    var pixelw = (domain[1] - domain[0]) / WIDTH * 1000000; // pixel width in nanoseconds
    var currpt;
    var prevpt;
    var offset = self.idata.offset;
    var lineChunks;
    var points;
    var currLineChunk;
    var pw;
    var dataObj;
    var j;
    var continueLoop;

    for (var i = 0; i < streams.length; i++) {
        xPixel = -Infinity;
        if (!data.hasOwnProperty(streams[i].uuid)) {
            s3ui.setStreamMessage(self, streams[i].uuid, "No data in specified time range", 3);
            continue;
        }
        lineChunks = [];
        points = [];
        currLineChunk = [[], [], []]; // first array is min points, second is mean points, third is max points
        streamdata = data[streams[i].uuid][1];
        pw = Math.pow(2, data[streams[i].uuid][2]);
        yScale = axisData[streamSettings[streams[i].uuid].axisid][2];
        startTime = domain[0].getTime() - offset;
        endTime = domain[1].getTime() - offset;
        startIndex = s3ui.binSearchCmp(streamdata, [startTime, 0], s3ui.cmpTimes);
        if (startIndex > 0 && s3ui.cmpTimes(streamdata[startIndex], [startTime, 0]) > 0) {
            startIndex--; // make sure to plot an extra data point at the beginning
        }
        outOfRange = true;
        continueLoop = true; // used to get one datapoint past the end
        for (j = startIndex; j < streamdata.length && continueLoop; j++) {
            continueLoop = (xPixel = xScale((currpt = streamdata[j])[0] + offset)) < WIDTH;
            prevpt = streamdata[j - 1];
            if (currLineChunk[0].length > 0 && (j == startIndex || (currpt[0] - prevpt[0]) * 1000000 + (currpt[1] - prevpt[1]) > pw)) {
                processLineChunk(currLineChunk, lineChunks, points);
                currLineChunk = [[], [], []];
            }
            // correct for nanoseconds
            xPixel += (currpt[1] / pixelw);
            mint = Math.min(Math.max(yScale(currpt[2]), -2000000), 2000000);
            currLineChunk[0].push(xPixel + "," + mint);
            currLineChunk[1].push(xPixel + "," + Math.min(Math.max(yScale(currpt[3]), -2000000), 2000000));
            maxt = Math.min(Math.max(yScale(currpt[4]), -2000000), 2000000);
            currLineChunk[2].push(xPixel + "," + maxt);
            outOfRange = outOfRange && (mint < 0 || mint > HEIGHT) && (maxt < 0 || maxt > HEIGHT) && (mint < HEIGHT || maxt > 0);
        }
        processLineChunk(currLineChunk, lineChunks, points);
        if ((lineChunks.length == 1 && lineChunks[0][0].length == 0) || streamdata[startIndex][0] > endTime || streamdata[j - 1][0] < startTime) {
            s3ui.setStreamMessage(self, streams[i].uuid, "No data in specified time range", 3);
        } else {
            s3ui.setStreamMessage(self, streams[i].uuid, undefined, 3);
        }
        color = streamSettings[streams[i].uuid].color;
        dataObj = {color: color, points: points, uuid: streams[i].uuid};
        dataObj.linechunks = lineChunks.map(function (x) {
                x[0].reverse();
                x[1] = x[1].join(" ");
                x[0] = x.pop().join(" ") + " " + x[0].join(" ");
                return x;
            });
        dataArray.push(dataObj);
        if (outOfRange) {
            s3ui.setStreamMessage(self, streams[i].uuid, "Data outside axis range; try rescaling y-axis", 2);
        } else {
            s3ui.setStreamMessage(self, streams[i].uuid, undefined, 2);
        }
    }
    update = d3.select(self.find("g.chartarea"))
      .selectAll("g.streamGroup")
      .data(dataArray);
        
    var enter = update.enter()
      .append("g")
      .attr("class", "streamGroup");
        
    setAppearance(self, drawFast ? enter : update);
        
    update.exit()
        .remove();
        
    var oldUpdate = update;
        
    update = update.selectAll("g")
      .data(function (d, i) { return dataArray[i].linechunks; });
      
    update.enter()
      .append("g");
      
    update.exit()
      .remove();
    
    update.selectAll("polyline").remove();
    
    update
      .append("polyline")
        .attr("class", "streamRange")
        .attr("points", function (d) { return d[0]; });
        
    update
      .append("polyline")
        .attr("class", "streamMean")
        .attr("points", function (d) { return d[1]; });
        
    update = oldUpdate
      .selectAll("circle.streamPoint")
      .data(function (d, i) { return dataArray[i].points; });
      
    update.enter()
      .append("circle")
      .attr("class", "streamPoint");
      
    update
        .attr("cx", function (d) { return d[0]; })
        .attr("cy", function (d) { return d[1]; })
        .attr("r", 1);
    
    update.exit().remove();
    
    if (!drawFast) {
        s3ui.updatePlotMessage(self);
    }
    
    if (self.idata.showingDensity != undefined && self.idata.oldData.hasOwnProperty(self.idata.showingDensity)) {
        s3ui.setStreamMessage(self, self.idata.showingDensity, "Interval width: " + s3ui.nanosToUnit(Math.pow(2, self.idata.oldData[self.idata.showingDensity][2])), 4);
        var ddplot = $(self.find("svg.chart g.data-density-plot"));
        ddplot.children("polyline, circle").remove();
        showDataDensity(self, self.idata.showingDensity);
    }
}

function setAppearance(self, update) {
    update
        .attr("class", function (dataObj) { return "streamGroup series-" + dataObj.uuid; })
        .attr("stroke", function (d) { return d.color; })
        .attr("stroke-width", function (d) { return self.idata.showingDensity == d.uuid ? 3 : 1; })
        .attr("fill", function (d) { return d.color; })
        .attr("fill-opacity", function (d) { return self.idata.showingDensity == d.uuid ? 0.5 : 0.3; });
}

function processLineChunk(lc, lineChunks, points) {
    if (lc[0].length == 1) {
        var minval = lc[0];
        var maxval = lc[2];
        var meanval = lc[1];
        if (minval[0] == maxval[0]) {
            meanval = meanval[0].split(",");
            points.push([parseFloat(meanval[0]), parseFloat(meanval[1])]);
        } else {
            var minv = minval[0].split(",");
            var mint = parseFloat(minv[0]);
            lc[0] = [(mint - 0.5) + "," + minv[1], (mint + 0.5) + "," + minv[1]];
            var meanv = meanval[0].split(",");
            var meant = parseFloat(meanv[0]);
            lc[1] = [(meant - 0.5) + "," + meanv[1], (meant + 0.5) + "," + meanv[1]];
            var maxv = maxval[0].split(",");
            var maxt = parseFloat(maxv[0]);
            lc[2] = [(maxt - 0.5) + "," + maxv[1], (maxt + 0.5) + "," + maxv[1]];
            lineChunks.push(lc);
        }
    } else {
        lineChunks.push(lc);
    }
}

function showDataDensity(self, uuid) {
    var oldShowingDensity = self.idata.showingDensity;
    self.idata.showingDensity = uuid;
    if (!self.idata.onscreen || !self.idata.oldData.hasOwnProperty(uuid)) {
        self.idata.drawnBefore = false;
        return;
    }
    if (oldShowingDensity != uuid || !self.idata.drawnBefore) {
        $("g.series-" + uuid).attr({"stroke-width": 3, "fill-opacity": 0.5});
    }
    self.idata.drawnBefore = true;
    var domain = self.idata.oldXScale.domain();
    var streamdata = self.idata.oldData[uuid][1];
    var j;
    var selectedStreams = self.idata.selectedStreams;
    for (j = 0; j < selectedStreams.length; j++) {
        if (selectedStreams[j].uuid == uuid) {
            break;
        }
    }
    var WIDTH = self.idata.WIDTH;
    var pixelw = (domain[1] - domain[0]) / WIDTH;
    var pw = Math.pow(2, self.idata.oldData[uuid][2]);
    pixelw *= 1000000;
    var offset = self.idata.offset
    var startTime = domain[0].getTime() - offset;
    var totalmax = 0;
    var xPixel;
    var prevIntervalEnd;
    var toDraw = [];
    var lastiteration;
    var startIndex;
    var prevpt;
    var oldXScale = self.idata.oldXScale;
    if (streamdata.length > 0) {    
        var i;
        startIndex = s3ui.binSearchCmp(streamdata, [startTime, 0], s3ui.cmpTimes);
        if (startIndex < streamdata.length && s3ui.cmpTimes(streamdata[startIndex], [startTime, 0]) < 0) {
            startIndex++;
        }
        if (startIndex >= streamdata.length) {
            startIndex = streamdata.length - 1;
        }
        totalmax = streamdata[startIndex][5];
        lastiteration = false;
        for (i = startIndex; i < streamdata.length; i++) {
            xPixel = oldXScale(streamdata[i][0] + offset);
            xPixel += ((streamdata[i][1] - pw/2) / pixelw);
            if (xPixel < 0) {
                xPixel = 0;
            }
            if (xPixel > WIDTH) {
                xPixel = WIDTH;
                lastiteration = true;
            }
            if (i == 0) {
                prevpt = [self.idata.oldData[uuid][3], 0, 0, 0, 0, 0];
            } else {
                prevpt = streamdata[i - 1];
            }
            if (((streamdata[i][0] - prevpt[0]) * 1000000) + streamdata[i][1] - prevpt[1] <= pw) {
                if (i != 0) { // if this is the first point in the cache entry, then the cache start is less than a pointwidth away and don't drop it to zero
                    if (i == startIndex) {
                        toDraw.push([Math.max(0, Math.min(WIDTH, oldXScale(prevpt[0] + offset))), prevpt[5]]);
                    }
                    toDraw.push([xPixel, toDraw[toDraw.length - 1][1]]);
                }
            } else {
                prevIntervalEnd = Math.max(0, Math.min(WIDTH, (oldXScale(prevpt[0] + offset) + ((prevpt[1] + (pw/2)) / pixelw)))); // x pixel of end of previous interval
                if (prevIntervalEnd != 0) {
                    toDraw.push([prevIntervalEnd, prevpt[5]]);
                }
                toDraw.push([prevIntervalEnd, 0]);
                toDraw.push([xPixel, 0]);
            }
            if (!(streamdata[i][5] <= totalmax)) {
                totalmax = streamdata[i][5];
            }
            if (lastiteration) {
                break;
            } else {
                toDraw.push([xPixel, streamdata[i][5]]);
            }
        }
        if (i == streamdata.length && (self.idata.oldData[uuid][4] - streamdata[i - 1][0]) * 1000000 - streamdata[i - 1][1] >= pw) {
            // Force the plot to 0
            toDraw.push([toDraw[toDraw.length - 1][0], 0]);
            // Keep it at zero for the correct amount of time
            var lastpixel = oldXScale(self.idata.oldData[uuid][4] + offset);
            if (lastpixel > 0) {
                toDraw.push([Math.min(lastpixel, WIDTH), 0]);
            }
        }
    }
    if (toDraw.length == 0) { // streamdata is empty, OR nothing relevant is there to draw
        toDraw = [[Math.max(0, oldXScale(self.idata.oldData[uuid][3] + offset)), 0], [Math.min(WIDTH, oldXScale(self.idata.oldData[uuid][4] + offset)), 0]];
        totalmax = 0;
    }
    var yScale;
    if (totalmax == 0) {
        totalmax = 1;
    }
    
    yScale = d3.scale.linear().domain([0, totalmax]).range([45, 0]);
    for (j = 0; j < toDraw.length; j++) {
        if (toDraw[j][0] == 0 && j > 0) {
            toDraw.shift(); // Only draw one point at x = 0; there may be more in the array
            j--;
        }
        toDraw[j][1] = yScale(toDraw[j][1]); // this does linear scale
    }
    var ddplot = d3.select(self.find("svg.chart g.data-density-plot"));
    if (toDraw.length == 1) {
        if (toDraw[0][0] != 0) {
            ddplot.append("circle")
                .attr("class", "density-" + uuid)
                .attr("cx", toDraw[0][0])
                .attr("cy", toDraw[0][1])
                .attr("r", 1)
                .attr("fill", self.idata.streamSettings[uuid].color);
        }
    } else {
        ddplot.append("polyline")
            .attr("class", "density-" + uuid)
            .attr("points", toDraw.join(" "))
            .attr("fill", "none")
            .attr("stroke", self.idata.streamSettings[uuid].color);
    }
        
    var formatter = d3.format("f");
    
    ddplot.select("g.data-density-axis")
        .call(d3.svg.axis().scale(yScale).orient("left").tickValues([0, Math.round(totalmax / 2), totalmax]).tickFormat(formatter));
}

function hideDataDensity(self) {
    var ddplot = $(self.find("svg.chart g.data-density-plot"));
    ddplot.children("polyline, circle").remove();
    ddplot.children("g.data-density-axis").empty();
    $("svg.chart g.series-" + self.idata.showingDensity).attr({"stroke-width": 1, "fill-opacity": 0.3});
    self.idata.showingDensity = undefined;
}

function resetZoom(self) {
    self.idata.zoom.scale(1)
        .translate([0, 0]);
    if (self.idata.onscreen) {
        repaintZoomNewData(self);
    }
}

function applyDisplayColor(self, axisObj, streamSettings) {
    var color = s3ui.getDisplayColor(axisObj, streamSettings);
    yAxisDOMElem = self.find(".drawnAxis-" + axisObj.axisid);
    yAxisDOMElem.style.fill = color;
    $(yAxisDOMElem.querySelectorAll("line")).css("stroke", color);
    yAxisDOMElem.querySelector("path").style.stroke = color;
    self.find(".axistitle-" + axisObj.axisid).style.fill = color;
}

s3ui.init_plot = init_plot;
s3ui.repaintZoomNewData = repaintZoomNewData;
s3ui.updateSize = updateSize;
s3ui.updatePlot = updatePlot;
s3ui.applySettings = applySettings;
s3ui.showDataDensity = showDataDensity;
s3ui.hideDataDensity = hideDataDensity;
s3ui.resetZoom = resetZoom;
s3ui.applyDisplayColor = applyDisplayColor;
