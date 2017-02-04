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

function init_streamtree(self) {
    self.idata.streamTree = undefined;
    self.idata.$streamTreeDiv = undefined;
    self.idata.rootNodes = undefined;; // Acts as a set of root nodes: maps name to id
    self.idata.leafNodes = undefined; // Acts as a set of leaf nodes that appear in the tree: maps full path to id
    self.idata.loadingRootNodes = undefined; // Acts as a set of root nodes that are loading (used to prevent duplicate requests)
    self.idata.pendingStreamRequests = 0; // Keeps track of the number of requests for stream objects that are currently open (currently not used, but may be useful for optimizing)
    self.idata.internallyIgnoreSelects = false;
    self.idata.initiallySelectedStreams = {}; // Maps the source name of a stream to an object that maps path to stream object, if it has not yet been found in the tree
    self.idata.mayHaveSelectedLeaves = undefined; // An array of IDs of root nodes whose children may be initially selected
    self.idata.numOtherLoadables = undefined;
    self.idata.numLeaves = undefined;
}

function handleFailedLoad(description) {
    if (description.length != 0) {
        alert("Could not load node contents: " + description);
    } else {
        alert("Could not load node contents");
    }
}

/* When a user logs in or logs out, we need to update the stream tree.
   Unlike updateStreamList, this maintains, to the best of its ability,
   which streams are selected. */
function updateStreamTree(self) {
    var curr_state = s3ui.createPermalink(self, true);
    s3ui.updateStreamList(self);
    if (curr_state != undefined) {
        s3ui.executePermalink(self, curr_state, true); // reselect the streams from before, to the best of our ability
    }
}

function updateStreamList(self) {
    self.idata.rootNodes = {};
    self.idata.leafNodes = {};
    self.idata.loadingRootNodes = {};
    self.idata.numOtherLoadables = 0;
    self.idata.numLeaves = 0;
    self.idata.mayHaveSelectedLeaves = [];

    if (self.idata.streamTree != undefined) {
        // Remove everything from legend before destroying tree
        var roots = self.idata.streamTree.get_node("#").children;
        for (i = 0; i < roots.length; i++) {
            s3ui.selectNode(self, self.idata.streamTree, false, self.idata.streamTree.get_node(roots[i]));
        }
        self.idata.$streamTreeDiv.off();
        self.idata.streamTree.destroy(true);
        s3ui.applySettings(self, false);
    }

    var streamTreeDiv = $(self.find("div.streamTree"));
    self.idata.$streamTreeDiv = streamTreeDiv;

    streamTreeDiv.on("loaded.jstree", function (event, data) {
            for (var i = self.idata.mayHaveSelectedLeaves.length - 1; i >= 0; i--) {
                streamTree.load_node(self.idata.mayHaveSelectedLeaves[i]);
            }
        });
    streamTreeDiv.on("select_node.jstree", function (event, data) {
            if (self.idata.internallyIgnoreSelects) {
                event.stopImmediatePropagation();
                return;
            }
            selectNode(self, streamTree, true, data.node);
            s3ui.applySettings(self, true);
        });
    streamTreeDiv.on("deselect_node.jstree", function (event, data) {
            selectNode(self, streamTree, false, data.node);
            s3ui.applySettings(self, false);
        });

    streamTreeDiv.on("click", ".jstree-checkbox", function (event) {
            var id = event.target.parentNode.parentNode.getAttribute("id");
            var node = streamTree.get_node(id);
            if (streamTree.is_selected(node)) {
                streamTree.deselect_node(node);
            } else {
                streamTree.checkbox_select_node(node);
            }
            return false;
        });

    streamTreeDiv.jstree({
            core: {
                data: function (obj, callback) {
                        if (obj.id == "#") {
                            self.requester.makeTreeTopRequest(function (data) {
                                    var sourceList = data;
                                    var i;
                                    var rootID;
                                    for (i = 0; i < sourceList.length; i++) {
                                        rootID = "root_" + i;
                                        self.idata.rootNodes[sourceList[i]] = rootID;
                                        sourceList[i] = function (sourceName) {
                                                if (self.idata.initiallySelectedStreams.hasOwnProperty(sourceName)) {
                                                    self.idata.mayHaveSelectedLeaves.push(rootID);
                                                }
                                                return {
                                                        id: rootID,
                                                        /* This is pathological, but what if the source name is <script> ... </script>? */
                                                        text: s3ui.escapeHTMLEntities(sourceName),
                                                        data: {
                                                                toplevel: true,
                                                                children: function (callback) {
                                                                        self.requester.makeTreeBranchRequest(sourceName, function (data) {
                                                                                callback.call(this, pathsToTree(self, sourceName, data, function (treebranch) {
                                                                                        return function (callback) {
                                                                                                self.requester.makeTreeLeafRequest(treebranch, function (data) {
                                                                                                        callback.call(this, pathsToTree(self, treebranch, data, null));
                                                                                                    }, function (jqXHR) {
                                                                                                        handleFailedLoad(jqXHR.responseText);
                                                                                                        callback.call(this, []);
                                                                                                    });
                                                                                            };
                                                                                    }));
                                                                            }, function (jqXHR) {
                                                                                handleFailedLoad(jqXHR.responseText);
                                                                                callback.call(this, []);
                                                                            });
                                                                    },
                                                                child: false
                                                            },
                                                        children: true
                                                    };
                                            }(sourceList[i]);
                                    }
                                    callback(sourceList);
                                }, function (jqXHR) {
                                    handleFailedLoad(jqXHR.responseText);
                                    callback([]);
                                });
                        } else {
                            obj.data.children(callback);
                        }
                    }
            },
            contextmenu: {
                select_node: false,
                items: function (node, callback) { return getContextMenu(self, node, callback); },
                show_at_node: false
            },
            plugins: ["checkbox", "contextmenu", "sort"]
        });
    var streamTree = $.jstree.reference(streamTreeDiv);
    self.idata.streamTree = streamTree;

    /* I'm using a hack to intercept a "select_node.jstree" event
       before it occurs, in the case where the user cancels it before
       it is complete. */
    streamTree.old_select_node = streamTree.select_node;
    streamTree.select_node = makeSelectHandler(self, streamTree, false);

    streamTree.checkbox_select_node = makeSelectHandler(self, streamTree, true); // select all children when checkbox is clicked, but just expand if text is clicked
}

function needToLoad(node) {
    return (node.id.lastIndexOf("root_", 0) === 0 || node.id.lastIndexOf("other_", 0) == 0) && node.children.length == 0;
}

/* If SELECTALLCHILDREN is true, selects all the children. If not, simply expands the node. */
function makeSelectHandler(self, streamTree, selectAllChildren) {
    var MAX_NODE_COUNT = 50;
    var NOTICE_NODE_COUNT = 5;
    var handler = function (nodes, suppress_event, prevent_open, skipCountPrompt) {
            skipCountPrompt = skipCountPrompt || false;
            if (nodes.length == undefined) {
                nodes = [nodes];
            }
            var node;
            var streamCount;
            var dorecursiveselection = function (streamCount) {
                    if (streamCount !== undefined) {
                        if (streamCount > MAX_NODE_COUNT) {
                            if (!confirm("This action will select more than " + MAX_NODE_COUNT + " streams, potentially causing instability and rendering problems. Continue?")) {
                                return
                            }
                        } else if (streamCount > NOTICE_NODE_COUNT) {
                            if (!confirm("About to select " + streamCount + " streams. Continue?")) {
                                return
                            }
                        }
                    }
                    if (needToLoad(node)) {
                        if (!self.idata.loadingRootNodes[node.id]) {
                            self.idata.loadingRootNodes[node.id] = true;
                            streamTree.load_node(node, function (node, status) {
                                    self.idata.loadingRootNodes[node.id] = false;
                                    if (status) {
                                        if (selectAllChildren) {
                                            if (!streamTree.is_selected(node)) {
                                                handler(node, suppress_event, true, true);
                                            }
                                        } else {
                                            streamTree.toggle_node(node);
                                        }
                                    } else {
                                        handleFailedLoad(status);
                                    }
                                });
                        }
                    } else if (selectAllChildren) {
                        if (node.children.length == 0) {
                            streamTree.old_select_node(node, suppress_event, prevent_open); // if it's a leaf, select it
                        } else {
                            handler(node.children, suppress_event, true, true);
                        }
                    } else {
                        if (node.children.length == 0) {
                            streamTree.old_select_node(node, suppress_event, prevent_open); // if it's a leaf, select it
                        } else {
                            streamTree.toggle_node(node);
                        }
                    }
                };
            for (var i = 0; i < nodes.length; i++) {
                node = streamTree.get_node(nodes[i]);
                if (skipCountPrompt) {
                    dorecursiveselection(undefined);
                } else {
                    countUnselectedStreamsAsync(streamTree, node, MAX_NODE_COUNT + 1, dorecursiveselection);
                }
            }
        };
    return handler;
}


// The following functions are useful for the tree for selecting streams

/* Converts a list of paths into a tree (i.e. a nested object
 * structure that will work with jsTree). Returns the nested tree structure
 * and an object mapping uuid to node in a 2-element array.
 *
 * If LOADNEXT is null or undefined, then the terminating elements of the
 * provided paths are assumed to be streams. Otherwise, it must be a function
 * that takes a path as its sole argument and returns a function suitable
 * for loading additional data past the terminating elements of the provided
 * paths.
 */
function pathsToTree(self, pathPrefix, streamList, loadNext) {
    var rootNodes = []; // An array of root nodes
    var rootCache = {}; // A map of names of sources to the corresponding object

    var path;
    var hierarchy;
    var currNodes;
    var currCache;
    var levelName;
    var childNode;
    for (var i = 0; i < streamList.length; i++) {
        /* For each path we got back, parse the path. */
        path = streamList[i];
        hierarchy = path.split("/");
        /* The paths come with a leading slash ("/") that we need to ignore,
         * hence the call to splice below.
         */
        hierarchy.splice(0, 1);
        currNodes = rootNodes;
        currCache = rootCache;
        for (var j = 0; j < hierarchy.length; j++) {
            levelName = hierarchy[j];
            /* For each level of the path (except for the root), we need to add
             * that node to the list of its parent's children.
             */
            if (currCache.hasOwnProperty(levelName)) {
                /* We hit this case when we have two paths like a/b/c and
                 * a/b/d. When we look at the second path, b is already a child
                 * of a, so we just want to use b. We do NOT want to create a
                 * new node for b and add d as a child of that new node.
                 */
                currNodes = currCache[levelName].children;
                currCache = currCache[levelName].childCache;
            } else {
                /* Construct the node entry corresponding to this level of the
                 * path in question and add it to the parent node.
                 */
                childNode = {
                    text: s3ui.escapeHTMLEntities(levelName),
                    children: [],
                    childCache: {},
                    data: { toplevel: false, child: false } // the documentation says I can add additional properties directly, but that doesn't seem to work
                };
                currNodes.push(childNode);
                currCache[levelName] = childNode;
                currNodes = childNode.children;
                currCache = childNode.childCache;
                if (j == hierarchy.length - 1) {
                    var fullPath = pathPrefix + path;
                    if (loadNext == null) {
                        /* This is a node representing a stream: a leaf in the stream tree. */
                        var parts = s3ui.splitPath(fullPath);
                        var sourceName = parts[0];
                        var path = parts[1];
                        childNode.id = "leaf_" + self.idata.numLeaves++;
                        self.idata.leafNodes[fullPath] = childNode.id;
                        childNode.data.path = path;
                        childNode.icon = false;
                        childNode.data.selected = false;
                        childNode.data.sourceName = sourceName;
                        childNode.data.child = true;
                        var initiallySelectedStreams = self.idata.initiallySelectedStreams;
                        if (initiallySelectedStreams.hasOwnProperty(sourceName) && initiallySelectedStreams[sourceName].hasOwnProperty(path)) {
                            childNode.data.streamdata = initiallySelectedStreams[sourceName][path];
                            initiallySelectedStreams[sourceName].count--;
                            if (initiallySelectedStreams[sourceName].count == 0) {
                                delete initiallySelectedStreams[sourceName];
                            } else {
                                delete initiallySelectedStreams[sourceName][path];
                            }
                            childNode.data.selected = true;
                            childNode.state = { selected: true };
                        }
                    } else {
                        /* We need to load more data when this node is expanded... */
                        childNode.id = "other_" + self.idata.numOtherLoadables++;
                        childNode.data.children = loadNext(fullPath);
                        childNode.children = true;
                    }
                }
            }
        }
    }

    return rootNodes;
}

/* Given a node, determines the options available to it. */
function getContextMenu(self, node, callback) {
    if (node.data.child) {
        return {
                showInfo: {
                        label: "Show Info",
                        action: function () {
                                if (node.data.streamdata == undefined) {
                                    self.requester.makeMetadataFromLeafRequest(node.data.sourceName + node.data.path, function (data) {
                                             if (node.data.streamdata == undefined) {
                                                 // Used to be data = JSON.parse(data)[0] but I removed the extra list around it
                                                 node.data.streamdata = data;
                                             }
                                             alert(s3ui.getInfo(node.data.streamdata, "\n", false));
                                         }, function (jqXHR) {
                                             handleFailedLoad(jqXHR.responseText);
                                         });
                                } else {
                                    alert(s3ui.getInfo(node.data.streamdata, "\n", false));
                                }
                            }
                    }
            };
    } else {
        return {
                expandcontract: {
                        label: self.idata.streamTree.is_closed(node) ? "Expand" : "Collapse",
                        action: function () {
                                self.idata.streamTree.select_node(node);
                            }
                    },
                select: {
                        label: "Select",
                        action: function () {
                                self.idata.streamTree.checkbox_select_node(node);
                            }
                    },
                deselect: {
                        label: "Deselect",
                        action: function () {
                                self.idata.internallyIgnoreSelects = true;
                                self.idata.streamTree.old_select_node(node, true, true);
                                self.idata.internallyIgnoreSelects = false;
                                self.idata.streamTree.deselect_node(node);
                            }
                    }
            };
    }
}

/* Selects or deselects a node and all of its children, maintaining internal
   state ONLY (not outward appearance of being checked!). Returns true if a
   POST request was made for at least one stream object; otherwise returns
   false. */
function selectNode(self, tree, select, node) { // unfortunately there's no simple way to differentiate between undetermined and unselected nodes
    if (!node.data.child) {
        var result = false;
        for (var i = 0; i < node.children.length; i++) {
            result = selectNode(self, tree, select, tree.get_node(node.children[i])) || result;
        }
        return result;
    } else if (node.data.selected != select) {
        node.data.selected = select;
        if (node.data.streamdata === undefined && select) {
            self.idata.pendingStreamRequests += 1;
            self.requester.makeMetadataFromLeafRequest(node.data.sourceName + node.data.path, function (data) {
                    self.idata.pendingStreamRequests -= 1;
                    if (node.data.selected == select) { // the box may have been unchecked in the meantime
                        if (node.data.streamdata == undefined) { // it might have been loaded in the meantime
                            // Used to be data = JSON.parse(data)[0] but I removed the extra list around it
                            node.data.streamdata = data;
                        }
                        s3ui.toggleLegend(self, select, node.data.streamdata, true);
                    }
                }, function (jqXHR) {
                    self.idata.pendingStreamRequests -= 1;
                    handleFailedLoad(jqXHR.responseText);
                    tree.deselect_node(node);
                });
            return true;
        } else if (node.data.streamdata !== undefined) {
            s3ui.toggleLegend(self, select, node.data.streamdata, true);
            return false;
        } else {
            /* If the streamdata isn't loaded and we're deselecting, it's because
             * the node failed to load. So we silently deselect it.
             */
            return true;
        }
    }
}

/* Counts the total number of unselected streams in a node, making data
 * requests where necessary to load nodes, until the count reaches a set
 * maximum.
 */
function countUnselectedStreamsAsync(tree, node, maximum, callback) {
    if (node.data.child) {
        callback(node.data.selected ? 0 : 1);
        return
    }

    var count = 0;
    if (needToLoad(node)) {
        tree.load_node(node, function (loadednode, status) {
                if (status) {
                    countUnselectedStreamsAsync(tree, node, maximum, callback);
                } else {
                    handleFailedLoad(status);
                }
            });
    } else {
        var total = 0;
        var i = 0;
        var countChildrenFromI = function () {
                countUnselectedStreamsAsync(tree, tree.get_node(node.children[i]), maximum, function (count) {
                        total += count;
                        i++;
                        if (i >= node.children.length || total >= maximum) {
                            callback(total);
                        } else {
                            /* Use setTimeout to avoid unbounded stack growth. */
                            setTimeout(countChildrenFromI, 0);
                        }
                    });
            };
        countChildrenFromI();
    }
}

s3ui.init_streamtree = init_streamtree;
s3ui.updateStreamList = updateStreamList;
s3ui.updateStreamTree = updateStreamTree;
s3ui.selectNode = selectNode;
