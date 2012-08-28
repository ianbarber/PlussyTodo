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
	"appengine/urlfetch"
	"fmt"
	"strings"
	"time"
	"crypto/md5"
	"encoding/json"
	"html/template"
	"net/http"
	plushistory "code.google.com/p/google-api-go-client/plus/v1moments"
	"code.google.com/p/google-api-go-client/plus/v1"
	"code.google.com/p/goauth2/oauth"
	//"code.google.com/p/google-api-go-client/googleapi"
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

type UserEntry struct {
	Tok oauth.Token
	Name string
	Id string
	Imurl string
}

/* Main entry function on app engine */
func init() {
	http.HandleFunc("/", home)
	http.HandleFunc("/list", managelist)
	http.HandleFunc("/list/", manageitem)
	if !appengine.IsDevAppServer() {
		config.RedirectURL = "http://plussytodo.appspot.com/"
	}
}

/**
 ** Web Handler Functions
 **/
 
/* Render the main screen */
func home(w http.ResponseWriter, r *http.Request) {
	var user UserEntry
	var url string
	var lists []TodoList = getdefaultlists()
	
	ctx := appengine.NewContext(r)
	user = getuser(w, r, ctx)
	
	// If cookie, look up token 
	if r.FormValue("code") != "" {
		t := &oauth.Transport{
				Config: config,
				Transport: &urlfetch.Transport{Context: ctx},
			}
		token, err := t.Exchange(r.FormValue("code"))
		if err != nil {
			http.Error(w, "OAuth Error: " + err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Retrieve profile information
		oauthClient := t.Client()
		svc, err := plus.New(oauthClient)
		profile, err := svc.People.Get("me").Do()
		if err != nil {
			http.Error(w, "Plus API Error: " + err.Error(), http.StatusInternalServerError)
			return
		}		
		
		// Generate ID
		h := md5.New()
		h.Write([]byte(profile.Id + "blah"))
		key := fmt.Sprintf("%x", h.Sum(nil))
		
		user = UserEntry{
			Tok: *token,
			Name: profile.DisplayName,
			Imurl: profile.Image.Url,
			Id: profile.Id,
		}
		
		serialised, err := json.Marshal(user)
		if err != nil {
			http.Error(w, "JSON Error: " + err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Store token in datastore
		item := &memcache.Item{
		    Key:   key,
		    Value: serialised,
		}
		
		err = memcache.Set(ctx, item)
		if err != nil {
			http.Error(w, "Memcache Error: " + err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Set as session ID cookie
		cookie := &http.Cookie{
			Value: key,
			Name: cookieName,
		}
		http.SetCookie(w, cookie)
	} 
	
	if user.Id == "" {
		url = config.AuthCodeURL("");
	} else {
		for i := range lists {
			lists[i].getitems(ctx, user.Id)
		}
	}
	
	homeTemplate.Execute(w, map[string]interface{}{"URL": url, "User": user, "Lists": lists})
}

/* View or add to list */
func managelist(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	user := getuser(w, r, ctx)

	if r.Method == "POST" && user.Id != "" {
		/* If we're POSTing, create a new item */
		todo := TodoItem{
			User:       user.Id,
			Entry:      r.FormValue("entry"),
			Date:       time.Now(),
			Type:       r.FormValue("type"),
			IsComplete: false,
		}

		url, err := todo.store(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Push moment to history
		pushmoment(ctx, user, url, "http://schemas.google.com/CreateActivity")
		//ctx.Infof("Attempting to add moment : %v", url)
		
		http.Redirect(w, r, url, http.StatusFound)
	} else {
		http.Error(w, "No resource to retrieve", http.StatusNotFound)
	}
}

/* View or update specific todos */
func manageitem(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	/* Retrieve the item we're working with */
	itemp, err := getitem(ctx, r.URL.Path)
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
		user := getuser(w, r, ctx)
		if user.Id != item.User {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		err = item.checkoff(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Push moment to history
		pushmoment(ctx, user, fmt.Sprintf("%s%s", baseurl(), r.URL.Path), "http://schemas.google.com/AddActivity")
		//ctx.Infof("Attempting to delete moment : %v", fmt.Sprintf("%s%s", baseurl(), r.URL.Path))
	}

	/* Otherwise, just render the item */
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

/* Push a moment to the Google+ history of the authenticated user */
func pushmoment(ctx appengine.Context, user UserEntry, url string, moment_type string) error {
	t := &oauth.Transport{
			Config: config,
			Transport: &urlfetch.Transport{Context: ctx},
		}
	t.Token = &oauth.Token{AccessToken: user.Tok.AccessToken}

	svc, err := plushistory.New(t.Client())

	target := plushistory.ItemScope {
		Url: url,
	}

	moment := plushistory.Moment {
		Target: &target, 
		Type: moment_type,
	}
	
	_, err = svc.Moments.Insert("me", "vault", &moment).Do()
	
	/*mom, err := googleapi.WithoutDataWrapper.JSONReader(moment)
	resp, err := svc.Moments.Insert("me", "vault", &moment).Debug(true).Do()
	
	ctx.Infof("%s %#v %#v", mom, resp, err);*/
	
	return err
}

/* Retrieve a user based on a cookie */
func getuser(w http.ResponseWriter, r *http.Request, ctx appengine.Context) UserEntry {
	var user UserEntry
	
	cookie, _ := r.Cookie(cookieName) 	
	if cookie != nil {
		item, _ := memcache.Get(ctx, cookie.Value); 
		if item != nil {
			// Unmarshal JSON
			json.Unmarshal(item.Value, &user)
		}
	}
	
	return user
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

var itemTemplate = template.Must(template.ParseFiles("todo/templates/entry.html"))
var plusTemplate = template.Must(template.ParseFiles("todo/templates/moment_plus.html"))

var homeTemplate = template.Must(template.ParseFiles("todo/templates/main.html"))
var itemsTemplate = template.Must(homeTemplate.ParseFiles("todo/templates/list.html"))
var entryTemplate = template.Must(homeTemplate.ParseFiles("todo/templates/entry.html"))

var config = &oauth.Config{
        ClientId:     "212575495446.apps.googleusercontent.com",
        ClientSecret: "5AG6EYykbjUMb3vAHpS0asy4",
        Scope:        plus.PlusMeScope + " " + plushistory.PlusMomentsWriteScope, 
        AuthURL:      "https://accounts.google.com/o/oauth2/auth",
        TokenURL:     "https://accounts.google.com/o/oauth2/token",
		RedirectURL:  "http://localhost:8080/",
}
