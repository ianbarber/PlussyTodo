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

var STORE_KEY = 'plussytodo-auth';
var auth;
var user;

// Require the other modules
require(["jquery", "gplusclient", "plusone"]);

/* 	This function is triggered when we sign in with the G+ button, and so 
	is the main entry point into our application. We need to check the 
	auth result, then retrieve information about the user */
function onSignInCallback(authResult) {

	// If authentication didn't succeed, just bail out.
	if (authResult.error != null) {
		alert("Sign In Failed");
		$('#spinner').hide();
		$('#signin').show();
		return;
	}
	time = new Date();

	// Store auth object to local storage. 
	localStorage.setItem(STORE_KEY, JSON.stringify({
		auth: authResult,
		time: time.getTime()
	}));

	auth = authResult;
	gapi.auth.setToken(auth);

	// Retrieve the user's profile information
	var args = {
		'path': '/plus/v1/people/me?fields=displayName,id,image',
		'method': 'GET',
		'callback': function(response) {
			// We can now display our todo list, and personalise the page
			$("H1").html(response.displayName + "'s Todos");
			$("H1").css("background-image", 'url(' + response.image.url + ')');
			$("H1").addClass('signedin');

			// In case there was any question about this being a demo
			user = response.id;

			// Get rid of the loading spinner
			$('#spinner').hide();
			$('#signin').hide();

			// Retrieve a list of TODOs
			getItems(user);
		}
	};
	gapi.client.request(args);
}

/* Retrieve the items for the user from our appengine app */

function getItems(email) {
	$.ajax({
		url: "/list",
		cache: false,
		data: {
			user: email
		}
	}).done(function(html) {
		// We retrieve HTML from the app, so we can add that
		$("#listcontainer").append(html);

		// Set up the event handler for the 'done' click
		$("body").delegate(".rem", "click", checkOff);

		// Setup the event handler for the new click
		$(".add-to-do").submit(function(e) {
			e.preventDefault();
			type = $(this).attr("data-itemname");

			// Add the todo to the remote storage
			entry = $("#" + type + "-entry").val();
			storeItem(user, type, entry);

			// Blank the box
			$("#" + type + "-entry").val("");
		});
	});
}

/* 	This function stores the todo to the backend, 
	and pushes a new moment to the Google+ history
	API */
function storeItem(user, type, entry) {
	// Call out to the backend
	$.post("/list", {
		user: user,
		type: type,
		entry: entry
	}, function(link, status, jqXHR) {
		// Add the LI
		$("#" + type + "-todo-items").append(link);

		// We pass the real location as a custom X-Header, as
		// there's a redirect involved
		url = jqXHR.getResponseHeader("X-Item-Location");

		// Make the Create moment
		moment = {
			"type": "http://schemas.google.com/CreateActivity",
			"target": {
				"url": url
			}
		};

		// Finally, we create the moment and push it to +
		pushMoment(moment);
	});
}

/* This function marks a task as 'done' and pushes a moment to + */

function checkOff(e) {
	e.preventDefault();
	// We store the item details as data- attributes
	var itemid = $(this).attr("data-itemid");
	var itemtype = $(this).attr("data-itemtype");

	$.post("/list/" + itemid + "/type/" + itemtype, {
		user: user
	}, function(done, status, jqXHR) {
		// Remove the LI
		$("#" + itemid).fadeOut(function() {
			$("#" + itemid).remove();
		});

		// Get the URL
		url = jqXHR.getResponseHeader("X-Item-Location");

		// We use an AddActivity as there's no better matching type
		moment = {
			"type": "http://schemas.google.com/AddActivity",
			"target": {
				"url": url
			}
		};

		pushMoment(moment);
	});
}

/* This function adds the moment to the History API */
function pushMoment(moment) {
	gapi.auth.setToken(auth);

	var args = {
		'path': '/plus/v1moments/people/me/moments/vault?debug=true',
		'method': 'POST',
		'body': JSON.stringify(moment),
		'callback': function(response) {
			// We probably would want to check for errors here!
		}
	};
	gapi.client.request(args);
}
