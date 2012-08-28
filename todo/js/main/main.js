/*
Copyright 2012 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Require the other modules
require(["jquery", "plusone"], function() {
	// Set up the event handler for the 'done' click
	$("body").delegate(".rem", "click", checkOff);

	// Setup the event handler for the new click
	$(".add-to-do").submit(function(e) {
		e.preventDefault();
		type = $(this).attr("data-itemname");

		// Add the todo to the remote storage
		entry = $("#" + type + "-entry").val();
		storeItem(type, entry);

		// Blank the new item box
		$("#" + type + "-entry").val("");
	});
});

/* 	This function stores the todo to the backend, 
	and pushes a new moment to the Google+ history
	API */
function storeItem(type, entry) {
	// Call out to the backend
	$.post("/list", {
		type: type,
		entry: entry
	}, function(link, status, jqXHR) {
		// Add the LI
		$("#" + type + "-todo-items").append(link);
	});
}

/* This function marks a task as 'done' and pushes a moment to + */

function checkOff(e) {
	e.preventDefault();
	// We store the item details as data- attributes
	var itemid = $(this).attr("data-itemid");
	var itemtype = $(this).attr("data-itemtype");

	$.post("/list/" + itemid + "/type/" + itemtype, null, 
	function(done, status, jqXHR) {
		// Remove the LI
		$("#" + itemid).fadeOut(function() {
			$("#" + itemid).remove();
		});
	});
}