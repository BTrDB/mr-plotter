timezoneJS.timezone.zoneFileBasePath = "timezone-js/tz";

/* Provide a generalizable asynchronous callback API. */
timezoneJS._initialized = false;
timezoneJS._callbacks = [];
timezoneJS.timezone.init({ callback: function () {
        timezoneJS._initialized = true;
        timezoneJS._process_callbacks();
    } });
    
timezoneJS.isInitialized = function () {
        return this._initialized;
    };
    
timezoneJS.addCallback = function (cb) {
        this._callbacks.push(cb);
    };
    
timezoneJS._process_callbacks = function () {
        for (var i = 0; i < this._callbacks.length; i++) {
            this._callbacks[i]();
        }
    };
