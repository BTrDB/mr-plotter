// This code is to be run on the Meteor server (all other files in this package are to be run on the client)

s3ui_permalinks = new Meteor.Collection("s3ui_permalinks");
csv_downloads = new Meteor.Collection("csv_downloads");

// Apparently there is a bug in Iron-Router that requires this fix
Router.onBeforeAction(Iron.Router.bodyParser.urlencoded({
        extended: false
    }));
    
var Future = Npm.require('fibers/future');
var fut = new Future();

s3ui_server = {};
s3ui_server.createPermalink = function (permalinkJSON) {
                permalinkJSON.created = (new Date()).getTime();
                permalinkJSON.lastAccessed = "never";
                return s3ui_permalinks.insert(permalinkJSON);
            };
            
permalink_schema = {
    autoupdate: {"boolean": null},
    axes: {"object": null},
    resetStart: {"number": null},
    resetEnd: {"number": null},
    tz: {"string": null},
    dst: {"boolean": null},
    streams: {"object": null},
    window_type: {"string": null},
    window_width: {"number": null},
    start: {"number": null},
    end: {"number": null},
    vertCursor1: {"number": null},
    vertCursor2: {"number": null},
    horizCursor1: {"number": null},
    horizCursor2: {"number": null}
};

stream_schema = {
    stream: {"string": null, "object": null},
    color: {"string": null},
    selected: {"boolean": null}
};

axis_schema = {
    axisname: {"string": null},
    streams: {"object": null},
    scale: {"object": null},
    rightside: {"object": null, "boolean": null}
};

required_properties = ["streams"];

axis_required_properties = ["axisname", "streams", "scale", "rightside"];

conditional_properties = {
    "fixed": ["start", "end"],
    "last": ["window_width"],
    "now": ["window_width"]
};

connection_pool = {};

var net = Npm.require("net");
var http = Npm.require('http');

var connPools = {};

Meteor.onConnection(function (connection) {
        var pool = new http.Agent();
        pool.maxSockets = 25;
        connPools[connection.id] = pool;
        connection.onClose(function () {
                // It seems I don't have to do any cleanup work to delete an http.Agent
                for (var i = 0; i < pool.sockets.length; i++) {
                    pool.sockets[i].end();
                    pool.sockets[i].destroy();
                }
                delete connPools[connection.id];
            })
    });

function getConnection(dataURLStart) {
    if (connection_pool.hasOwnProperty(dataURLStart)) {
        if (connection_pool[dataURLStart].length > 0) {
            return connection_pool[dataURLStart].pop();
        }
    } else {
        connection_pool[dataURLStart] = [];
    }
    var parsedDataURL = url.parse(dataURLStart);
    var socket = net.connect({
            host: parsedDataURL.hostname,
            port: parsedDataURL.port == null ? undefined : parsedDataURL.port,
            local: parseDataURL.path
        });
}
            
Meteor.methods({
        processQuery: function (query, type) {
                this.unblock();
                var params = query.split(" ");
                var url, payload, request;
                if (params[0] === "SENDPOST") {
                    url = params[1];
                    payload = params.slice(2).join(' ');
                    request = "POST";
                } else {
                    url = params[0];
                    payload = '';
                    request = "GET";
                }
                try { 
                    var result = HTTP.call(request, url, {
                            content: payload
                        });
                    return result.content;
                } catch (err) {
                    console.log(query);
                    console.log(err);
                    return '[]';
                }
            },
        requestMetadata: function (request, urlbase) {
                this.unblock();
                var user = Meteor.user();
                var url;
                if (user == null || !user.hasOwnProperty('s3ui_tags')) {
                    url = urlbase +'?tags=public';
                } else {
                    url = urlbase + '?tags=' + user.s3ui_tags.join(',');
                }
                var result = HTTP.call('POST', url, {
                        content: request
                    });
                return result.content;
            },
        createPermalink: s3ui_server.createPermalink,
        retrievePermalink: function (permalinkID) {
                var obj = s3ui_permalinks.findOne({"_id": permalinkID});
                if (obj == undefined) {
                    obj = null;
                } else {
                    s3ui_permalinks.update(permalinkID, {$set: {lastAccessed: (new Date()).getTime()}});
                }
                return obj;
            },
        requestData: function (dataUrl) {
                this.unblock();
                var clientIP = this.connection.clientAddress;
                var fut = new Future();
                var parsedURL = url.parse(dataUrl);
                
                var req = http.request({
                        hostname: parsedURL.hostname,
                        port: parsedURL.port == null ? undefined : parseInt(parsedURL.port),
                        path: parsedURL.path,
                        headers: {'x-forwarded-for':clientIP},
                        agent: connPools[this.connection.id],
                        method: "GET"
                    }, function (resp) {
                        fut.return(resp);
                    });
                req.end();
                response = fut.wait();
                
                var fut2 = new Future();
                var dataBuffer = [];
                response.on('data', function (data) {
                        dataBuffer.push(data.toString());
                    });
                response.on('end', function () {
                        fut2.return(dataBuffer);
                    });
                var getData = Meteor.wrapAsync(response.on, response);
                var data = fut2.wait().join('');
                return data;
            }
    });
    
var url = Npm.require('url');
    
Router.map(function () {
        this.route('csv_forwarder', {
                path: '/s3ui_csv',
                where: 'server',
                action: function () {
                        this.response.statusCode = 200;
                        this.response.setHeader('Content-Disposition', 'attachment; filename=data.csv');
                        this.response.setHeader('Content-Type', 'text/plain; charset=utf-8');
                        this.response.setHeader('Transfer-Encoding', 'chunked');
                        if (this.request.method != "POST") {
                            this.response.write("To get a CSV file, send the required data as a JSON document via a POST request.");
                            this.response.end();
                            return;
                        }
                        mainresponse = this.response;
                        var parsedURL = url.parse(this.request.body.dest);
                        var csvRequest = http.request({
                                host: parsedURL.hostname,
                                port: parsedURL.port == null ? undefined : parsedURL.port,
                                path: parsedURL.path,
                                method: "POST",
                                headers: {"Content-type": "application/x-www-form-urlencoded"}
                            }, function (result) {
                                result.on('data', function (chunk) {
                                        mainresponse.write(chunk.toString());
                                    });
                                result.on('end', function (chunk) {
                                        mainresponse.end();
                                    });
                            });
                        csvRequest.on("error", function (e) {
                                console.log("Error in CSV request: " + e);
                            });
                        csvRequest.end('body=' + this.request.body.body);
                    }
            });
        this.route('permalink_generator', {
                path: '/s3ui_permalink',
                where: 'server',
                action: function () {
                        this.response.statusCode = 400;
                        this.response.setHeader('Content-Type', 'text/plain');
                        if (this.request.method != "POST") {
                            this.response.write("To create a permalink, send the data as a JSON document via a POST request. Use the following format:\n\npermalink_data=<JSON>");
                            this.response.end();
                            return;
                        }
                        var jsonPermalink = this.request.body.permalink_data;
                        var id, property, i, stream;
                        if (jsonPermalink == undefined) {
                            this.response.write("Error: required key 'permalink_data' is not present");
                            console.log(JSON.stringify(this.response.body))
                            console.log(this.response.body);
                            this.response.write(JSON.stringify(this.response.body));
                            this.response.end();
                        } else {
                            try {
                                jsonPermalink = JSON.parse(jsonPermalink);
                            } catch (exception) {
                                this.response.write("Error: received invalid JSON: " + exception);
                                this.response.end();
                                return;
                            }
                            // validate the schema
                            if (check_extra_fields(jsonPermalink, permalink_schema, this.response)) {
                                return;
                            }
                            // check that required fields are present
                            if (check_required_fields(jsonPermalink, required_properties, this.response)) {
                                return;
                            }
                            
                            // check that streams are valid
                            var streams = jsonPermalink.streams;
                            for (i = 0; i < streams.length; i++) {
                                stream = streams[i];
                                if (stream == undefined) {
                                    this.response.write("Error: streams must be an array");
                                    this.response.end();
                                    return;
                                }
                                if (check_extra_fields(stream, stream_schema, this.response)) {
                                    return;
                                }
                                if (!stream.hasOwnProperty("stream")) {
                                    this.response.write("Error: required field stream is missing in element of streams array");
                                    this.response.end();
                                    return;
                                }
                                if (stream.stream == null) {
                                    this.response.write("Error: stream data for an element of streams array is null");
                                    this.response.end();
                                    return;
                                }
                                if (stream.hasOwnProperty("color")) {
                                    if (stream.color.length != 7 || stream.color.charAt(0) != '#') {
                                        this.response.write("Error: stream color must be a string containing the pound sign (#) and a six digit hexadecimal number");
                                        this.response.end();
                                        return;
                                    }
                                }
                            }
                            // check that axes are valid
                            var axes, axis, j;
                            if (jsonPermalink.hasOwnProperty("axes")) {
                                axes = jsonPermalink.axes;
                                for (i = 0; i < axes.length; i++) {
                                    axis = axes[i];
                                    if (check_extra_fields(axis, axis_schema, this.response)) {
                                        return;
                                    }
                                    if (check_required_fields(axis, axis_required_properties, this.response)) {
                                        return;
                                    }
                                    if (typeof axis.rightside == "object" && axis.rightside != null) {
                                        this.response.write("Error: rightside field of element of axes field may be only true, false, or null")
                                        this.response.end();
                                        return;
                                    }
                                    if (!Array.isArray(axis.streams)) {
                                        this.response.write("Error: streams field of element of axes field must be an array");
                                        this.response.end();
                                        return;
                                    }
                                    for (j = 0; j < axis.streams; j++) {
                                        if (typeof axis.streams[j] != "string") {
                                            this.response.write("Error: streams field of element of axes field must be a string");
                                            this.response.end();
                                            return;
                                        }
                                    }
                                    if (!Array.isArray(axis.scale) || axis.scale.length != 2 || typeof axis.scale[0] != "number" || typeof axis.scale[1] != "number") {
                                        this.response.write("Error: scale field of element of axes field must be an array of length 2 containing numbers");
                                        this.response.end();
                                        return;
                                    }
                                }
                            }
                            // check that conditional fields are present
                            var window_type = jsonPermalink.window_type;
                            if (window_type == undefined) {
                                window_type = "fixed";
                            }
                            if (!conditional_properties.hasOwnProperty(window_type)) {
                                this.response.write("Error: " + window_type + " is an invalid window_type");
                                this.response.end();
                                return;
                            }
                            var additional_properties = conditional_properties[window_type];
                            for (i = 0; i < additional_properties.length; i++) {
                                if (!jsonPermalink.hasOwnProperty(additional_properties[i])) {
                                    this.response.write("Error: window_type " + window_type + " requires " + additional_properties[i] + " to be specified");
                                    this.response.end();
                                    return;
                                }
                            }
                            try {
                                id = Meteor.call('createPermalink', jsonPermalink);
                            } catch (exception) {
                                this.response.write("Error: document could not be inserted into Mongo database: " + exception);
                                this.response.end();
                                return;
                            }
                            this.response.statusCode = 200;
                            this.response.write(id);
                            this.response.end();
                        }
                    }
            });
    });
    
function check_extra_fields (object, schema, response) {
    for (var property in object) {
        if (object.hasOwnProperty(property)) {
            // screen for invalid properties and bad types
            if (!schema.hasOwnProperty(property)) {
                response.write("Error: " + property + " is not a valid field");
                response.end();
                return true; 
            } else if (!schema[property].hasOwnProperty(typeof object[property])) {
                response.write("Error: " + property + " must be of one of the following types: " + Object.keys(schema[property]).join(", "));
                response.end();
                return true;
            }
        }
    }
    return false;
}

function check_required_fields (object, required_properties, response) {
    for (var i = 0; i < required_properties.length; i++) {
        if (!object.hasOwnProperty(required_properties[i])) {
            response.write("Error: required field " + required_properties[i] + " is missing");
            response.end();
            return true;
        }
    }
    return false;
}
