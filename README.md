Mr. Plotter
===========
The Multi-Resolution Plotter is a utility for viewing data in BTrDB.

The backend consists of three parts: (1) an HTTP server written in Go, (2) a
metadata server that answers sMAP queries, and (3) a Mongo Database to store
permalinks and account information. The script accounts.py provides a REPL
for creating accounts and granting permissions. Permissions are granted
to users at the granularity of "tags". The script metadata.py is a minimal
implementation of the metadata server; tagconfig.json maps "tags" to viewable
streams, and must be supplied to metadata.py as a command line argument.

Instantiating a Graph
---------------------
To insert a graph with full control available to the user, simply execute the
following in Javascript:
<pre><code>mr_plotter(domElement);</code></pre>
The graph will be inserted as a child of the specified DOM Element.

One can also instantiate the graph with parameters:
<pre><code>mr_plotter(domElement, storageKey, options, callback1, callback2, backend);</code></pre>

STORAGEKEY is a string that is the key to use when persisting login sessions.
Local storage is used by default. cookies are used when local storage is not
available. Two keys are stored: _storageKey_\_username and _storageKey_\_token.
OPTIONS is a Javascript object containing parameters (explained below).
CALLBACK1 and CALLBACK2 are callback functions invoked during initialization
(explained below). BACKEND is the URL of the backend; if omitted, it is derived
from window.location.

OPTIONS, the object of parameters, may have the following properties (all optional):

* hide\_permalink - TRUE if the button to generate a permalink and the space to display it are to be hidden. Defaults to FALSE.
* hide\_graph\_export - TRUE if the menu to export the graph to an SVG file is to be hidden. Defaults to FALSE.
* hide\_stream\_legend - TRUE if the legend displaying streams is to be hidden. Defaults to FALSE.
* hide\_axis\_legend - TRUE if the legend displaying axes is to be hidden. Defaults to FALSE.
* hide\_automatic\_update - TRUE if the checkbox specifying whether stream removals and axis changes should be applied automatically is to be hidden. Defaults to FALSE.
* hide\_login\_button - TRUE if the "Login" dropdown menu is to be hidden. Defaults to FALSE.
* hide\_apply\_button - TRUE if the "Apply and Plot" button is to be hidden. Defaults to FALSE.
* hide\_reset\_button - TRUE if the "Reset Zoom" button is to be hidden. Defaults to FALSE.
* hide\_autozoom\_button - True if the "Autozoom and Plot" button is to be hiddn. Defaults to FALSE.
* hide\_info\_bar - TRUE if the area where general messages are displayed is to be hidden. Defaults to FALSE.
* hide\_time\_selection - TRUE if the menu to select the start and end times is to be hidden. Defaults to FALSE.
* hide\_stream\_tree - TRUE if the tree used to select streams is to be hidden. Defaults to FALSE.
* hide\_plot\_directions - TRUE if the directions for how to use the interface are to be hidden. Defaults to FALSE.
* hide\_streamtree\_options - TRUE if the "Refresh Tree" and "Deselect All" buttons are to be hidden. Defaults to FALSE.
* hide\_axis\_selection - TRUE if the axis selection menu within the legend is to be hidden. Defaults to FALSE.
* disable\_color\_selection - TRUE if the color selection menu within the legend is to be disabled. Defaults to FALSE.
* permalinkStart - Specifies the start of the permalink URL. Defaults to the current window location of the browser, excluding any seach queries in the URL, but including the question mark.
* width - A function that returns the targed width of the graph (_not_ just the chart area) to use. Defaults to a function that sizes the graph to the well it is in ("div.chartContainer"). If you plan to override this with a custom setting, the s3ui.pixelsToInt helper function may be of interest to you.
* widthmin - The minimum width, in pixels, of the width of the chart area (_not_ the whole graph). Defaults to 450.
* height - Specifies the height of the chart area (_not_ the whole graph). Defaults to 300.
* queryLow - The earliest time, in milliseconds since the epoch, when data can be queried. Defaults to 0. queryHigh - queryLow should be at least 2 ms, and queryLow must be at least 0 for correct functionality.
* queryHigh - The latest time, in milliseconds since the epoch, when data can be queried. Defaults to 3458764513820. queryHigh - queryLow should be at least 2 ms.
* pweHigh - The highest point width exponent with which data can be queried.
* bracketInterval - The approximate number of milliseconds at which to poll the server for a change in the time of the last data point. Note that the server is only polled for new data when the previously received point of the last data, and that the real time between samples will be slightly larger than the number specified here (i.e., this is a lower bound on the time between samples). Defaults to 5000.

When the graph has been displayed, but before any interactivity is added, the
first callback function is invoked with a single argument, namely the template
instance. The callback function is the mechanism through which the template
instance is made available. The template instance can be used to
programmatically change the settings (useful when settings have been hidden
from the user but still need to be manipulated) and even change some of the
parameters the graph was instantiated with (see below). The first callback
function can also be used to make simple changes to the DOM allowing for a
customized layout.

The second callback function is called when the tree of streams is initialized,
not fully loaded. Here, streams and settings can be selected programmatically.

The defaults for the callbacks are included in the global s3ui object as
s3ui.default\_cb1 and s3ui.default\_cb2. The default first callback makes the
left column in the default layout resizable. The default second callback reads
the page's URL and executes a permalink if present. These callback functions
can be overriden by instantiating the graph with different functions. If
one desires to use options (see above) but keep the callback functions the
same, one can simply put s3ui.default\_cb1 and s3ui.default\_cb2 into the
array. One can also do a partial override, i.e. use a function that calls the
default callback and does additional work.

The purpose of the callbacks is to allow you to perform actions after the plot
is fully loaded. In particular, you should make no assumptions about how much
of the plot is fully loaded after the mr_plotter function returns.

Custom Layouts
--------------
Simply including the s3plot template provides a default layout, and the first
callback function allows for simple layout changes (see above). But for more
advanced layouts it may be easier to write the layout in HTML. The
implementation of the plotter is, to a large degree, robust to layout changes;
it relies mostly on the classes of the HTML components, as long as the logical
components are kept together. For example, it is OK to move the login
section of the plotter's HTML to a different place, but it is not OK to put
the different components of the login dropdown menu in different places. The
"modules" that can be rearranged on the page are marked with HTML comments. If
you do not want all of the features of the graph, you _must_ still include all
of the components; use the appropriate parameters on instantiation to hide the
components you do not need (see above).

As a rule of thumb, any element in the "s3plot" template that has a class not
provided by Bootstrap contributes to the functioning of the graph in some way
and should be kept even if the layout is rearranged. The

Programmatically Changing the Graph
-----------------------------------
The "idata" property of the plotter instance is an object that stores the
instance fields  of the object (i.e., the variables used by the graph to
keep track of its internal state). The "imethods" property of the template
instance contains bound methods that can be used to programmatically manipulate
the state of the graph. The only exceptions to this rule are that the
"requester" object, which handles outstanding requests to the site's backend,
the storage key provided as an argument upon construction, the backend URL,
and some of the arguments provided upon construction, are direct properties of
the plotter instance, and therefore are in neither idata nor imethods.

The bound methods provided are:

* selectStreams(data\_lst) - Given DATA\_LST, a list of stream objects, selects the corresponding streams (works as long as the tree is initialized, even if no streams are loaded yet).
* deselectStreams(data\_lst) - Given DATA\_LST, a list of stream objects, deselects the corresponding streams (works as long as the tree is initialized, even if no streams are loaded yet).
* setStartTime(date) - Given a DATE object, sets the start time to the date it represents in local time.
* setEndTime(date) - Given a DATE object, sets the end time to the date it represents in local time.
* setTimezone(iana\_str) - Sets the timezone to IANA\_STR.
* setDST(dst) - Marks Daylight Savings Time as being in effect if DST is true. Otherwise, marks Daylight Savings Time as not being in effect.
* addAxis() - Creates a new y-axis and returns the id associated with it.
* removeAxis(id) - Removes the axis with the specified ID, reassigning streams as necessary. The axis with the id "y1" cannot be removed.
* renameAxis(id, newName) - Assigns the name NEWNAME to the axis with the specified ID.
* setAxisSide(id, leftOrRight) - Sets the side of the chart area where the axis with the specified ID will be displayed. If LEFTORRIGHT is true, it is set to the left side; otherwise it is set to the right side.
* setAxisScale(id, low, high) - Sets the scale of the axis with the specifed ID to have the specified LOW and HIGH values. If one of LOW and HIGH is undefined, only the other endpoint of the interval is set; if both are undefined, the "Autoscale" feature is used for that axis (exactly as if the "Autoscale" button were pressed).
* setStreamAxis(uuid, id) - Assigns the stream with the specifed UUID to the axis with the specified ID.
* setStreamColor(uuid, color) - Sets the color for the stream with the specified UUID to COLOR.
* applyAllSettings() - Programmatically clicks the "Apply All Settings and Update Plot" button.
* resetZoom() - Programmatically clicks the "Reset Zoom" button.
* toggleAutomaticUpdate() - Programmatically checks or unchecks the "Automatically apply stream removals and changes to axis settings" checkbox.
* applySettings() - Programmatically clicks the "Apply Settings" button (visible when "Automatically apply stream removals and changes to axis settings is unchecked")
* updateGraphSize() - Uses the width function to recompute the width of the graph.
* updateVerticalCursor(xCoord) - Creates a vertical cursor, if possible, at the location specified by XCOORD, in pixels to the right of the origin of the graph.
* updateHorizontalCursor(yCoord) - Creates a horizontal cursor, if possible, at the location specified by YCOORD, in pixels above the origin of the graph.
* toggleLegendEntrySelection(uuid) - Toggles selection of (i.e., selects or deselects, as appropriate) the entry in the legend corresponding to the stream with the specified UUID. If no such stream is in the legend, no action is taken.
* changeVisuals(options) - Reinitializes the visuals with the specified OPTIONS, according to the parameters specified (from the list above). The only differences between this function and the instantiation of the graph is that the "width" and "height" properties are ignored, and the new default values are those currently applied.

Permalinks
----------
The permalink feature allows one to save the state of the graph and load it
later using a generated permalink. To generate a permalink, click the "Generate
Permalink" button on the graph UI.

If you would prefer to generate a permalink programmatically by specifying the
settings of the graph rather than by creating the graph by hand, you should
instead send a POST request to {domain}/permalink. The payload of the
POST request should be a JSON object specifying the settings of the graph. The
schema of the JSON document is described below.

The permalink API allows three methods for specifying a permalink. A permalink
may be specified as "fixed", meaning that the start and end times are specified
in the number of nanoseconds since the UNIX epoch (Jan 1, 1970, UTC). A
permalink of type "now" always ends at the time the permalink is executed, and
starts a fixed number of nanoseconds before that. Finally, a permalink of type
"last" always ends at the last data point of the streams being displayed, and
starts a fixed number of nanoseconds before that.

In order that the graph be displayed correctly, the final start and end times
must be in the current UNIX epoch (i.e., Jan 01, 1970, UTC or later).

For the permalink, the following fields may be specified in the JSON object:

* autoupdate (optional) - Determines whether or nor the "Automatically apply settings" checkbox is checked. Defaults to TRUE.
* axes (optional) - A list of axis objects (see below for the schema of these objects) that specify the settings of the axes. If not specified, a reasonable default is chosen at runtime; all axes are on the left side of the graph, the axis names are just y1, y2, etc., streams are assigned to axes based on their units, and the scales of the axes are chosen based on the ranges of the streams assigned.
* resetStart (optional) - The start time that should be used when the "Reset Zoom" button is clicked. If not specified, a default value close to the start time initially displayed is chosen. For consistency, this value is specified in nanoseconds since the UNIX epoch in UTC time, but the value used is only precise up to the second.
* resetEnd (optional) - The end time that should be used when the "Reset Zoom" button is clicked. If not specified, a default value close to the end time initially displayed is chosen. For consistency, this value is specified in nanoseconds since the UNIX epoch in UTC time, but the value used is only precise up to the second.
* tz (optional) - The time zone in which the graph should be displayed. Defaults to "America/Los_Angeles". All dates in the permalink are specified in nanoseconds since the epoch in UTC time, regardless of the value of this parameter.
* dst (optional) - Indicates whether Daylight Savings Time is (TRUE) or is not (FALSE) in effect for the specified timezone.
* streams (required) - A list of objects specifying streams (see below for the schema of these objects).
* window\_type (optional) - A string specifying the type of window to be used. The possible values are "fixed", "last", and "now". If an invalid string is used or the key is omitted altogether, defaults to "fixed".
* window\_width (conditional) - The width of the window, specified in nanoseconds since the epoch (but precise to milliseconds). Required if the window\_type is "last" or "now", otherwise ignored.
* start (conditional) - The start time of the window, specified in nanoseconds since the epoch (but precise to milliseconds). Required if the window\_type if "fixed", otherwise ignored.
* end (conditional) - The start time of the window, specified in nanoseconds since the epoch (but precise to milliseconds). Required if the window\_type if "fixed", otherwise ignored.
* vertCursor1 (optional) - The position of the first vertical cursor, specified as the coordinate of the cursor on the horizontal axis divided by the range of the horizontal axis.
* vertCursor2 (optional) - The position of the first vertical cursor, specified as the coordinate of the cursor on the horizontal axis divided by the range of the horizontal axis.
* horizCursor1 (optional) - The position of the first horizontal cursor, specified as the coordinate of the cursor on a vertical axis divided by the length of the vertical axis.
* horizCursor2 (optional) - The position of the second horizontal cursor, specified as the coordinate of the cursor on a vertical axis divided by the length of the vertical axis.

The axis objects require the following fields:

* axisname - A string specifying the name of the axis.
* streams - An array of UUIDS of streams to be displayed on this axis.
* scale - A two element array specifying the lowest and highest values (in that order) to be displayed on this axis.
* rightside - Specifies where this axis should be drawn. TRUE means the axis should be drawn on the right side of the chart area, FALSE means the axis should be drawn on the left side of the chart area, and NULL means that the axis should be drawn in the graph.

The objects specifying streams may have the following fields:

* stream (required) - Specifies which stream should be drawn here, either as a UUID in the form of a string, or as an object containing all of the metadata of the stream.
* color (optional) - Specifies with what color the stream should be drawn, as a string with a pound sign (#) and a six-digit hexadecimal number specifying the color. This color _must_ be one of the colors that it is possible to pick with the color picker in the graph's UI. Defaults to one of the possible colors, depending of the stream's position in the legend.
* selected (optional) - Specifies if this stream is selected. Only one stream may be selected (if this field is set to TRUE for multiple streams, there is no guarantee as to which stream is actually selected).

Below is an example of permalink data that uses a fixed window:
<pre><code>{
	"autoupdate" : true,
	"axes" : [
		{
			"axisname" : "y1",
			"streams" : [
				"49129d4a-335e-4c81-a8a4-27f5d8c45646"
			],
			"scale" : [
				-1,
				1
			],
			"rightside" : false
		},
		{
			"axisname" : "y2",
			"streams" : [
				"571ce598-3ffd-499b-be6c-0df52e597c93"
			],
			"scale" : [
				-2,
				2
			],
			"rightside" : null
		}
	],
	"end" : 1408234686223000000,
	"resetEnd" : 1408746931000000000,
	"resetStart" : 1377210944000000000,
	"start" : 1408234676466000000,
	"streams" : [
		{
			"stream" : "49129d4a-335e-4c81-a8a4-27f5d8c45646",
			"color" : "#000000"
		},
		{
			"stream" : "571ce598-3ffd-499b-be6c-0df52e597c93",
			"color" : "#0000FF"
		}
	],
	"tz" : "America/Los_Angeles"
}</code></pre>

Below is an example of permalink data that uses a window of type "now" (a window of type "last" would be very similar):
<pre><code>{
	"autoupdate" : true,
	"axes" : [
		{
			"axisname" : "y1",
			"streams" : [
				"49129d4a-335e-4c81-a8a4-27f5d8c45646"
			],
			"scale" : [
				-1,
				1
			],
			"rightside" : false
		},
		{
			"axisname" : "y2",
			"streams" : [
				"571ce598-3ffd-499b-be6c-0df52e597c93"
			],
			"scale" : [
				-2,
				2
			],
			"rightside" : null
		}
	],
	"window_type" : "now",
	"window_width" : 60000000000,
	"resetEnd" : 1408746931000000000,
	"resetStart" : 1377210944000000000,
	"streams" : [
		{
			"stream" : "49129d4a-335e-4c81-a8a4-27f5d8c45646",
			"color" : "#000000"
		},
		{
			"stream" : "571ce598-3ffd-499b-be6c-0df52e597c93",
			"color" : "#0000FF"
		}
	],
	"tz" : "America/Los_Angeles"
}</code></pre>

Restricting Stream Access
-------------------------
It is possible to restrict streams to be visible only by certain users. The
accounts.py script provides a REPL that allows one to create user accounts,
and to associate which each user a set of tags. A _tag_ represents the
permissions to view a set of streams. Every user always has a tag called
_public_, which represents the ability to view publicly available streams;
additional tags may be added via the REPL.

The mapping from tags to specific streams that are viewable is in the file
tagconfig.json. Each tag is a key in the JSON documents; the corresponding
value is a list of path prefixes (as strings) that describe the streams
viewable by users with that tag.

If a user logs in or out while streams are selected, the plotting application
will maintain the streams being plotted as far as possible, ensuring that the
new user is still authorized to see those streams.

License
-------
Mr. Plotter is licensed under the GNU Affero General Public License.
