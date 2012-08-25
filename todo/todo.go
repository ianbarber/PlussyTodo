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

package todo

import (
	"appengine"
	"appengine/datastore"
	"appengine/memcache"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
	//"code.google.com/p/google-api-go-client/plus/v1moments"
	"code.google.com/p/goauth2/oauth"
)

type TodoItem struct {
	Id         string
	User       string
	Entry      string
	Date       time.Time
	IsComplete bool
	Type       string
	Key        *datastore.Key `datastore:"-"`
}

type TodoList struct {
	Name    string
	Title   string
	Imurl   string
	Done    string
	Pending string
	Items   []TodoItem
}

/* Main entry function on app engine */
func init() {
	http.HandleFunc("/", home)
	http.HandleFunc("/list", managelist)
	http.HandleFunc("/list/", manageitem)
}

/**
 ** Web Handler Functions
 **/

/* Render the main screen */
func home(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cookieName) 
	item, err := memcache.Get(c, cookie); 
	c := appengine.NewContext(r)
	
	// IF cookie, look up token 
	if item != nil {
		
	}
	else if r.FormValue("code") != "" {
		t := &oauth.Transport{Config: config}
		token, err := t.Exchange(r.FormValue("code"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Generate UUID
		key = 1; // TODO: Fix!
		
		// Store token in datastore
		item := &memcache.Item{
		    Key:   key,
		    Value: []byte(token),
		}
		
		// Set as session ID cookie
		memcache.Add(c, item)
		w.SetCookie(cookieName, key)
		
		//oauthClient := t.Client()
	} else {
		url := config.AuthCodeURL("");
		homeTemplate.Execute(w, map[string]interface{}{"URL": url})
	}
}

/* View or add to list */
func managelist(w http.ResponseWriter, r *http.Request) {
	context := appengine.NewContext(r)

	if r.Method == "POST" {
		/* If we're POSTing, create a new item */
		todo := TodoItem{
			User:       r.FormValue("user"),
			Entry:      r.FormValue("entry"),
			Date:       time.Now(),
			Type:       r.FormValue("type"),
			IsComplete: false,
		}

		url, err := todo.store(context)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
	} else {
		/* If GETing, retrieve the list of user todos */
		var lists []TodoList = getdefaultlists()
		for i := range lists {
			lists[i].getitems(context, r.FormValue("user"))
		}

		itemsTemplate.Execute(w, map[string]interface{}{"Lists": lists})
	}
}

/* View or update specific todos */
func manageitem(w http.ResponseWriter, r *http.Request) {
	context := appengine.NewContext(r)

	/* Retrieve the item we're working with */
	itemp, err := getitem(context, r.URL.Path)
	item := *itemp
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	/* If we're not getting it from ajax, switch to pretty template */
	if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
		itemlist := item.getlist()

		err = plusTemplate.Execute(w, map[string]interface{}{
			"IsComplete": item.IsComplete,
			"Type":       itemlist.Name,
			"Baseurl":    baseurl(),
			"Imurl":      itemlist.Imurl,
			"Entry":      item.Entry,
			"Done":       itemlist.Done,
			"Pending":    itemlist.Pending,
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		return
	}

	/* If we're POSTing to this, we're checking it off */
	if r.Method == "POST" {
		err = item.checkoff(context)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	/* Otherwise, just render the item */
	w.Header().Add("X-Item-Location", fmt.Sprintf("%s%s", baseurl(), r.URL.Path))
	err = itemTemplate.Execute(w, item)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/**
 ** TodoItem Methods
 **/

/* Update a todo item and mark as complete */
func (t *TodoItem) checkoff(context appengine.Context) error {
	todo := *t
	todo.IsComplete = true
	key := datastore.NewKey(context, "Todos-"+todo.Type, todo.Key.StringID(), todo.Key.IntID(), nil)
	_, err := datastore.Put(context, key, &todo)
	*t = todo
	return err
}

/* Retrieve the matching item list for a todo item */
func (t *TodoItem) getlist() TodoList {
	todo := *t
	var lists []TodoList = getdefaultlists()
	itemlist := lists[0]
	for i := range lists {
		if lists[i].Name == todo.Type {
			itemlist = lists[i]
			break
		}
	}
	return itemlist
}

/* Save a todo item */
func (t *TodoItem) store(context appengine.Context) (string, error) {
	todo := *t
	key, err := datastore.Put(context, datastore.NewIncompleteKey(context, "Todos-"+todo.Type, nil), &todo)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/list/%s/type/%s", baseurl(), key.Encode(), todo.Type)
	return url, nil
}

/**
 ** Todolist Methods
 **/

/* Retrieve a list of items for the todo list for a given user */
func (t *TodoList) getitems(context appengine.Context, user string) {
	tl := *t
	q := datastore.NewQuery("Todos-"+tl.Name).Filter("User =", user)
	q = q.Filter("IsComplete = ", false).Order("Date").Limit(1000)
	key, _ := q.GetAll(context, &tl.Items)
	for i := range tl.Items {
		tl.Items[i].Key = key[i]
	}
	*t = tl
}

/**
 ** Support Functions
 **/

/* Retrieve the appropriate base URL for the environment */
func baseurl() string {
	if appengine.IsDevAppServer() {
		return "http://localhost:8080"
	}
	return "http://plussytodo.appspot.com"
}

/* Given a URL path, retrieve the corresponding item */
func getitem(context appengine.Context, url string) (*TodoItem, error) {
	parts := strings.Split(strings.Replace(url, "/list/", "", 1), "/type/")
	id := parts[0]

	key, err := datastore.DecodeKey(id)
	if err != nil {
		return nil, err
	}

	item := new(TodoItem)
	err = datastore.Get(context, key, item)
	if err != nil {
		return nil, err
	}

	item.Key = key

	return item, nil
}

/* Get the default array lists */
func getdefaultlists() []TodoList {
	var lists []TodoList = make([]TodoList, 4)
	lists[0].Name = "work"
	lists[0].Title = "Work"
	lists[0].Imurl = "images/noun_project_1539"
	lists[0].Done = "A Completed Task"
	lists[0].Pending = "A Task To Do"
	lists[1].Name = "home"
	lists[1].Title = "Home"
	lists[1].Done = "A Completed Tas"
	lists[1].Pending = "A Task To Do"
	lists[1].Imurl = "images/noun_project_293"
	lists[2].Name = "calls"
	lists[2].Title = "Calls"
	lists[2].Done = "A Made Call"
	lists[2].Pending = "A Call To Make"
	lists[2].Imurl = "images/noun_project_127"
	lists[3].Name = "errands"
	lists[3].Title = "Errands"
	lists[3].Done = "A Completed Errand"
	lists[3].Pending = "An Errand To Run"
	lists[3].Imurl = "images/noun_project_3364"
	return lists
}

/**
 ** Templates
 **/

var cookieName = "SESSIONID"

var homeTemplate = template.Must(template.ParseFiles("todo/templates/main.html"))
var itemTemplate = template.Must(template.ParseFiles("todo/templates/entry.html"))
var plusTemplate = template.Must(template.ParseFiles("todo/templates/moment_plus.html"))
var itemsTemplate = template.Must(template.ParseFiles("todo/templates/list.html"))
var entryTemplate = template.Must(itemsTemplate.ParseFiles("todo/templates/entry.html"))

var config = &oauth.Config{
        ClientId:     "212575495446.apps.googleusercontent.com",
        ClientSecret: "5AG6EYykbjUMb3vAHpS0asy4",
        Scope:        "https://www.googleapis.com/auth/plus.me https://www.googleapis.com/auth/plus.moments.write", 
        AuthURL:      "https://accounts.google.com/o/oauth2/auth",
        TokenURL:     "https://accounts.google.com/o/oauth2/token",
		RedirectURL:  "http://localhost:8080/",
}
