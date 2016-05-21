/*
 * Copyright (C) 2016 Sam Kumar, Michael Andersen, and the University
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

s3ui = {instances: [], instanceid: -1}; // stores functions used in multiple files

function MrPlotter(container, storagekey, backend, options, cb1, cb2) {
    this.domelem = container;
    this.cookiekey = storagekey;
    this.backend = backend;
    
    /* In Meteor, this is where the constructor argument would go. I'm doing
       the same thing since the code assumes it would be like this. */
    this.data = [options, cb1, cb2];
    
    this.requester = new Requester(this, backend);
}

// Meteor provided these two functions on a template, to search inside of it
MrPlotter.prototype.find = function (expr) {
        return this.domelem.querySelector(expr);
    };

MrPlotter.prototype.$ = function (expr) {
        return $(this.domelem.querySelectorAll(expr));
    };
 
/** Instantiates Mr. Plotter as a child of the provided DOM element.
    BACKEND is the hostname/port of the backend (ex. localhost:8080).
    COOKIEKEY is the key to use for the cookie storing login data.
    OPTIONS is an object containing options to create the chart.
    CB1 and CB2 are callback overrides for the two callbacks, the first of
    which executes after the graph is built, and the second of which executes
    after the interactive features have been added. */
function mr_plotter(parent, storagekey, options, cb1, cb2, backend) {
    // This is the one place in the entire code that I use the ID of an element
    var template = document.getElementById("mrplotter");
    var container = document.createElement("div");
    if (backend == undefined) {
        backend = window.location.hostname + (window.location.port ? ":" + window.location.port : "");
    }
    try {
        a.b();
        container.appendChild(document.importNode(template.content, true));
    } catch (err) {
        container.innerHTML = template.innerHTML;
    }
    parent.appendChild(container);
    var instance = new MrPlotter(container, storagekey, backend, options, cb1, cb2);
    
    var initialize = function () { s3ui.__init__(instance); };
    
    // Wait until after timezoneJS has initialized before initializing the plot.
    if (timezoneJS.isInitialized()) {
        initialize();
    } else {
        timezoneJS.addCallback(initialize);
    }
    return instance;
}

s3ui.parsePixelsToInt = function (q) {
        return parseFloat(q.slice(0, q.length - 2));
    };

s3ui.default_cb1 = function (inst) {
        $(inst.find(".dispTable")).colResizable({
                    hoverCursor: "ew-resize",
                    dragCursor: "ew-resize",
                    minWidth: 0,
                    onResize: inst.imethods.updateGraphSize
                });
    };
    
s3ui.default_cb2 = function (inst) {
        if (window.location.search.length > 0) {
            s3ui.exec_permalink(inst, window.location.search.slice(1));
        }
    };

s3ui.exec_permalink = function (self, link_id) {
        self.requester.makePermalinkRetrievalRequest(link_id, function (result) {
                var permalinkJSON;
                if (result == undefined) {
                    return;
                } else if (result == "not found") {
                    console.log("Server could not retrieve data for permalink " + link_id);
                    return;
                }
                try {
                    permalinkJSON = JSON.parse(result);
                } catch (err) {
                    console.log('Invalid permalink response from server: ' + err);
                    return;
                }
                s3ui.executePermalink(self, permalinkJSON);
            });
    };
    
s3ui.__init__ = function (self) {
        s3ui.instances.push(self);
        
        self.idata = {}; // an object to store instance data
        self.imethods = {}; // an object to store instance methods
        
        self.idata.instanceid = ++s3ui.instanceid;
        if (s3ui.instanceid == 4503599627370496) {
            s3ui.instanceid = -4503599627370496;
        }
        
        $(self.find("div.streamLegend")).removeClass("streamLegend").addClass("streamLegend-" + self.idata.instanceid);
        self.idata.dynamicStyles = self.find("style.dynamicStyles");
        
        s3ui.init_axis(self);
        s3ui.init_plot(self);
        s3ui.init_data(self);
        s3ui.init_frontend(self);
        s3ui.init_streamtree(self);
        s3ui.init_control(self);
        s3ui.init_cursors(self);
        
        var c1, c2;
        
        if (typeof self.data[0] === "object") {
            init_visuals(self, self.data[0]);
            if (self.data[0].width != undefined) {
                self.idata.widthFunction = self.data[0].width;
            }
            if (self.data[0].widthmin != undefined) {
                self.idata.widthmin = self.data[0].widthmin;
            }
            if (self.data[0].height != undefined) {
                self.find("svg.chart").setAttribute("height", self.data[0].height + self.idata.margin.top + self.idata.margin.bottom);
                self.idata.HEIGHT = self.data[0].height;
            }
            if (self.data[0].permalinkStart != undefined) {
                self.idata.initPermalink = self.data[0].permalinkStart;
            }
            if (self.data[0].queryLow != undefined) {
                self.idata.queryLow = self.data[0].queryLow;
            }
            if (self.data[0].queryHigh != undefined) {
                self.idata.queryHigh = self.data[0].queryHigh;
            }
            if (self.data[0].pweHigh != undefined) {
                self.idata.pweHigh = self.data[0].pweHigh;
            }
            if (self.data[0].bracketInterval != undefined) {
                self.idata.bracketInterval = self.data[0].bracketInterval;
            }
            self.imethods.changeVisuals = function (options) {
                    init_visuals(self, options);
                };
        }
        
        if (typeof self.data[1] === "function") {
            c1 = self.data[1];
        } else {
            c1 = s3ui.default_cb1;
        }
        
        if (typeof self.data[2] === "function") {
            c2 = self.data[2];
        } else {
            c2 = s3ui.default_cb2;
        }
        
        init_graph(self, c1, c2);
    };
    
function init_visuals(self, options) {
    setVisibility(self, options, "div.permalinkGenerate", "hide_permalink");
    setVisibility(self, options, "div.graphExport", "hide_graph_export");
    setVisibility(self, options, "div.streamLegend-" + self.idata.instanceid, "hide_stream_legend");
    setVisibility(self, options, "div.axisLegend", "hide_axis_legend");
    setVisibility(self, options, "span.automaticUpdate", "hide_automatic_update");
    setVisibility(self, options, "div.plotButton", "hide_apply_button");
    setVisibility(self, options, "div.resetZoom", "hide_reset_button");
    setVisibility(self, options, "div.showAll", "hide_autozoom_button");
    setVisibility(self, options, "div.plotLoading", "hide_info_bar");
    setVisibility(self, options, "div.timeSelection", "hide_time_selection");
    setVisibility(self, options, "div.streamSelection", "hide_stream_tree");
    setVisibility(self, options, "div.login", "hide_login");
    setVisibility(self, options, "g.plotDirections", "hide_plot_directions");
    setVisibility(self, options, "div.streamTreeOptions", "hide_streamtree_options");
    
    setCSSRule(self, options, "tr.streamLegend-" + self.idata.instanceid + " select.axis-select { display: none; }", "hide_axis_selection");
    setCSSRule(self, options, "tr.streamLegend-" + self.idata.instanceid + " span.simplecolorpicker { pointer-events: none; }", "disable_color_selection");
}

function setVisibility(self, options, selector, attr) {
    if (options.hasOwnProperty(attr)) {
        if (options[attr]) {
            self.find(selector).setAttribute("style", "display: none;");
        } else {
            self.find(selector).setAttribute("style", "");
        }
    }
}

function setCSSRule(self, options, rule, attr) {
    if (options.hasOwnProperty(attr)) {
        var styles = self.idata.dynamicStyles;
        if (options[attr]) {
            styles.innerHTML += rule;
        } else {
            styles.innerHTML = styles.innerHTML.replace(rule, "");
        }
    }
}
    
function init_graph(self, c1, c2) {
    // Finish building the graph components
    s3ui.addYAxis(self);
    
    // first callback
    c1(self);
    
    // Make the window resize dynamically
    self.imethods.updateGraphSize();
    $(window).resize(self.imethods.updateGraphSize);
    
    // For some reason, Any+Time requires the text elements to have IDs.
    // So, I'm going to give them IDs that are unique across all instances
    self.find(".startdate").id = "start" + self.idata.instanceid;
    self.find(".enddate").id = "end" + self.idata.instanceid;
    
    // Event handlers are added programmatically
    self.find(".getPermalink").onclick = function () {
            setTimeout(function () {
                    if (s3ui.createPermalink(self, false) == undefined) {
                    	var permalinkElem = self.find(".permalink");
                        permalinkElem.innerHTML = 'You must plot some streams before creating a permalink.';
                    }
                }, 50);
        };
    self.find(".makeGraph").onclick = function () {
            self.find(".download-graph").innerHTML = 'Creating image...';
            setTimeout(function () { s3ui.createPlotDownload(self); }, 50);
        };
    var csvForm = self.find(".csv-form");
    csvForm.setAttribute("action", window.location.protocol + "//" + self.backend + "/csv");
    self.find(".makecsv").onclick = function () {
            s3ui.buildCSVMenu(self);
            $(self.find(".csv-modal")).modal("toggle");
        };
    self.find(".addAxis").onclick = function () {
            s3ui.addYAxis(self);
        };
    self.find(".plotButton").onclick = function () {
            self.idata.addedStreams = false;
            self.idata.changedTimes = false;
            self.idata.otherChange = false;
            s3ui.updatePlot(self);
            if (!self.idata.onscreen && !self.idata.pollingBrackets) {
                s3ui.startPollingBrackets(self);
            }
        };
    self.find(".resetZoom").onclick = function () {
            s3ui.resetZoom(self);
        };
    self.find(".showAll").onclick = function () {
            if (self.idata.selectedStreamsBuffer.length > 0) {
                self.imethods.resetZoom();
                var uuids = self.idata.selectedStreamsBuffer.map(function (s) { return s.uuid; });
                self.requester.makeBracketRequest(uuids, function (range) {
                        if (typeof(range) === "string") {
                            console.log("Autozoom error: " + range);
                            return;
                        }
                        if (range == undefined || range.Merged == undefined || range.Brackets == undefined) {
                            self.find(".plotLoading").innerHTML = "Error: Selected streams have no data.";
                            return;
                        }
                        s3ui.processBracketResponse(self, uuids, range);
                        range = range.Merged;
                        try {
                            var tz = s3ui.getSelectedTimezone(self);
                            var offset = 60000 * ((new Date()).getTimezoneOffset() - s3ui.getTimezoneOffsetMinutes(tz[0], tz[1]));
                            var naiveStart = new Date(range[0][0]);
                            var naiveEnd = new Date(range[1][0] + (range[1][1] > 0 ? 1 : 0));
                            self.imethods.setStartTime(new Date(naiveStart.getTime() + 60000 * (naiveStart.getTimezoneOffset() - s3ui.getTimezoneOffsetMinutes(tz[0], tz[1]))));
                            self.imethods.setEndTime(new Date(naiveEnd.getTime() + 60000 * (naiveEnd.getTimezoneOffset() - s3ui.getTimezoneOffsetMinutes(tz[0], tz[1]))));
                            self.imethods.applyAllSettings();
                        } catch (err) {
                            self.find(".plotLoading").innerHTML = err;
                        }
                    });
            } else {
                self.find(".plotLoading").innerHTML = "Error: No streams are selected.";
            }
        };
    self.find(".automaticAxisSetting").onchange = function () {
            self.idata.automaticAxisUpdate = !self.idata.automaticAxisUpdate;
            if (self.idata.automaticAxisUpdate) {
                this.parentNode.nextSibling.nextSibling.style.display = "none";
                self.idata.selectedStreams = self.idata.selectedStreamsBuffer;
                if (self.idata.otherChange || self.idata.addedStreams) {
                    self.idata.addedStreams = false;
                    self.idata.otherChange = false;
                    s3ui.applySettings(self, true);
                }
            } else {
                this.parentNode.nextSibling.nextSibling.style.display = "initial";
                s3ui.updatePlotMessage(self);
                self.idata.selectedStreamsBuffer = self.idata.selectedStreams.slice();
            }
        };
    self.find(".applySettingsButton").onclick = self.imethods.applySettings;
    var changedDate = function () {
            self.idata.changedTimes = true;
            s3ui.updatePlotMessage(self);
        };
    self.find(".startdate").onchange = changedDate;
    self.find(".enddate").onchange = changedDate;
    self.find(".m1yButton").onclick = function () {
            var m1y = new Date().getTime()-365*24*60*60*1000;
            self.$('.startdate').val(self.idata.dateConverter.format(new Date(m1y))).change();
        };
    self.find(".nowButton").onclick = function () {
            self.$('.enddate').val(self.idata.dateConverter.format(new Date())).change();
        };
    self.find(".timezoneSelect").onchange = function () {
            var visibility = (this[this.selectedIndex].value == 'OTHER' ? 'visible' : 'hidden');
            self.find(".otherTimezone").style.visibility = visibility;
            self.idata.changedTimes = true;
            s3ui.updatePlotMessage(self);
        };
    self.find(".dstButton").onclick = function () {
            self.idata.changedTimes = true;
            s3ui.updatePlotMessage(self);
        };
    self.find(".otherTimezone").onchange = changedDate;
    self.find(".refreshStreamTree").onclick = function () {
            s3ui.updateStreamTree(self);
        };
    self.find(".deselectStreamTree").onclick = function () {
            s3ui.updateStreamList(self);
        };
    
    self.$(".datefield").AnyTime_picker({format: self.idata.dateFormat});
    if (self.find(".automaticAxisSetting").checked) { // Some browsers may fill in this value automatically after refresh
        self.idata.automaticAxisUpdate = true;
        self.idata.selectedStreamsBuffer = self.idata.selectedStreams;
    } else {
        self.idata.automaticAxisUpdate = false;
        self.idata.selectedStreamsBuffer = [];
    }
    self.find(".timezoneSelect").onchange(); // In case the browser selects "Other:" after refresh
    self.idata.addedStreams = false;
    self.idata.changedTimes = false;
    self.idata.otherChange = false;
    s3ui.updatePlotMessage(self);
    
    // Make the login menu invisible, to start with
    self.$(".loginstate-start").hide()
    self.$(".loginstate-loggedin").hide();
    self.$(".loginstate-changepw").hide();
    
    // Buttons in login menu that perform actions
    self.find(".loginButton").onclick = function () {
            s3ui.login(self);
        };
    self.find(".logoffButton").onclick = function () {
            s3ui.logoff(self);
        };
    self.find(".changepwButton").onclick = function (event) {
            s3ui.changepw(self, event);
        };
        
    // Button to show the change password menu
    self.find(".changepwMenu").onclick = function (event) {
            s3ui.showChangepwMenu(self);
            event.stopPropagation(); // Prevents dropdown menu from disappearing
        };
        
    var $login = $(self.find(".login"));
    $login.on("hide.bs.dropdown", function () {
            if (self.idata.changingpw) {
                s3ui.hideChangepwMenu(self);
            }
        });
    $login.on("hidden.bs.dropdown", function () {
            self.find(".loginmessage").innerHTML = "";
        });
        
    s3ui.setLoginText(self, "Loading...");
    s3ui.checkCookie(self, function (username, token) {
            if (username === null) {
                s3ui.setLoginText(self, self.idata.defaultLoginMenuText);
                self.$(".loginstate-start").show()
            } else {
                s3ui.loggedin(self, username, token);
                self.$(".loginstate-loggedin").show();
            }
            
            s3ui.updateStreamTree(self)
            
            // Second callback
            if (typeof c2 == "function") {
                c2(self);
            }
        });
}
