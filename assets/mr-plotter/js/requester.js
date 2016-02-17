USE_WEBSOCKETS = false;

function DataConn(url) {
    this.ws = new WebSocket(url);
    this.openMessages = {};
    this.currMessage = 0;
    this.currResponse = null;
    this.ready = false;
    var self = this;
    this.ws.onopen = function () {
            self.ready = true;
        };
    this.ws.onmessage = function (response) {
            response = response.data;
            if (self.currResponse === null) {
                self.currResponse = response;
            } else {
                var callback = self.openMessages[response];
                delete self.openMessages[response];
                var response = self.currResponse;
                self.currResponse = null;
                callback(response);
            }
        };
}

DataConn.prototype.send = function(message, callback) {
    if (this.ready) {
        this.openMessages[this.currMessage] = callback;
        this.ws.send(message + "," + this.currMessage++);
        if (this.currMessage > 2000000) {
            this.currMessage = 0;
        }
    } else {
        console.log("WebSocket is not ready yet.");
    }
}

function Requester(backend) {
    this.backend = backend;
    this.token = "";
    if (USE_WEBSOCKETS) {
        this.dconnections = [];
        var i;
        for (i = 0; i < this.DATA_CONN; i++) {
            this.dconnections.push(new DataConn("wss://" + backend + "/dataws"));
        }
        this.bconnections = [];
        for (i = 0; i < this.BRACK_CONN; i++) {
        	this.bconnections.push(new DataConn("wss://" + backend + "/bracketws"));
        }
        this.currDConnection = 0;
        this.currBConnection = 0;
    }
}

Requester.prototype.DATA_CONN = 8;
Requester.prototype.BRACK_CONN = 2;

Requester.prototype.setToken = function (token) {
        if (token == undefined || token == null) {
            self.token = "";
        } else {
            self.token = token;
        }
    };

Requester.prototype.makeLoginRequest = function (username, password, success_callback, error_callback) {
        var loginjsonstr = JSON.stringify({"username": username, "password": password});
        return $.ajax({
            type: "POST",
            url: "https://" + this.backend + "/login",
            data: loginjsonstr,
            success: success_callback,
            dataType: "text",
            error: error_callback = undefined ? function () {} : error_callback
        });
    };
    
Requester.prototype.makeLogoffRequest = function (success_callback, error_callback) {
        return $.ajax({
            type: "POST",
            url: "https://" + this.backend + "/logoff",
            data: self.token,
            success: success_callback,
            dataType: "text",
            error: error_callback = undefined ? function () {} : error_callback
        });
    };
    
Requester.prototype.makeCheckTokenRequest = function (token, success_callback, error_callback) {
        return $.ajax({
            type: "POST",
            url: "https://" + this.backend + "/checktoken",
            data: token,
            success: success_callback,
            dataType: "text",
            error: error_callback = undefined ? function () {} : error_callback
        });
    };
    
Requester.prototype.makeChangePasswordRequest = function (old_password, new_password, success_callback, error_callback) {
        var changepwjsonstr = JSON.stringify({"token": self.token, "oldpassword": old_password, "newpassword": new_password});
        return $.ajax({
            type: "POST",
            url: "https://" + this.backend + "/changepw",
            data: changepwjsonstr,
            success: success_callback,
            dataType: "text",
            error: error_callback = undefined ? function () {} : error_callback
        });
    };

Requester.prototype.makeMetadataRequest = function (query, success_callback, error_callback) {
        return $.ajax({
                type: "POST",
                url: "https://" + this.backend + "/metadata",
                data: query.concat(self.token),
                success: success_callback,
                dataType: "text",
                error: error_callback == undefined ? function () {} : error_callback
            });
    };
    
Requester.prototype.makePermalinkInsertRequest = function (permalinkObj, success_callback, error_callback) {
        var permalinkjsonstr = JSON.stringify(permalinkObj)
        return $.ajax({
                type: "POST",
                url: "https://" + this.backend + "/permalink",
                data: permalinkjsonstr,
                success: success_callback,
                dataType: "text",
                error: error_callback = undefined ? function () {} : error_callback
            });
    };
    
Requester.prototype.makePermalinkRetrievalRequest = function (permalinkStr, success_callback, error_callback) {
        return $.ajax({
                type: "GET",
                url: "https://" + this.backend + "/permalink",
                data: {id: permalinkStr},
                success: success_callback,
                dataType: "text",
                error: error_callback = undefined ? function () {} : error_callback
            });
    };
    
Requester.prototype.makeDataRequest = function (request, success_callback, error_callback) {
		var request_str = request.join(',');
		if (USE_WEBSOCKETS) {
			if (!this.dconnections[this.currDConnection].ready) {
		    	var self = this;
		    	setTimeout(function () { self.makeDataRequest(request, success_callback, error_callback); }, 1000);
		    	return;
		    }
		    this.dconnections[this.currDConnection++].send(request_str, success_callback);
		    if (this.currDConnection == this.DATA_CONN) {
		        this.currDConnection = 0;
		    }
        } else {
            return $.ajax({
                    type: "POST",
                    url: "https://" + this.backend + "/data",
                    data: request_str,
                    success: success_callback,
                    dataType: "text",
                    error: error_callback == undefined ? function () {} : error_callback
                });
        }
    };
    
/** REQUEST should be an array of UUIDs whose brackets we want. */
Requester.prototype.makeBracketRequest = function (request, success_callback, error_callback) {
		var request_str = request.join(',');
		if (USE_WEBSOCKETS) {
		    if (!this.bconnections[this.currBConnection].ready) {
		    	var self = this;
		    	setTimeout(function () { self.makeBracketRequest(request, success_callback, error_callback); }, 1000);
		    	return;
		    }
		    this.bconnections[this.currBConnection++].send(request_str, success_callback);
		    if (this.currBConnection == this.BRACK_CONN) {
		        this.currBConnection = 0;
		    }
        } else {
            return $.ajax({
                    type: "POST",
                    url: "https://" + this.backend + "/bracket",
                    data: request_str,
                    success: success_callback,
                    dataType: "text",
                    error: error_callback == undefined ? function () {} : error_callback
                });
        }
	};
