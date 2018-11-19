// Copyright 2016 Jet Basrawi. All rights reserved.
//
// Use of this source code is governed by a permissive BSD 3 Clause License
// that can be found in the license file.

package goes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"

	"github.com/tomask-de/go.geteventstore"
	"github.com/tomask-de/go.geteventstore.testfeed"
	. "gopkg.in/check.v1"
)

var _ = Suite(&StreamWriterSuite{})

type StreamWriterSuite struct{}

func (s *StreamWriterSuite) SetUpTest(c *C) {
	setup()
}
func (s *StreamWriterSuite) TearDownTest(c *C) {
	teardown()
}

func (s *StreamWriterSuite) TestAppendEventsSingle(c *C) {
	data := &MyDataType{Field1: 445, Field2: "Some string"}
	et := "SomeEventType"
	ev := goes.NewEvent("", et, data, nil)
	streamName := "Some-Stream"
	url := fmt.Sprintf("/streams/%s", streamName)
	mux.HandleFunc(url, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, "POST")

		b, _ := ioutil.ReadAll(r.Body)
		se := []goes.Event{}
		err := json.NewDecoder(bytes.NewReader(b)).Decode(&se)
		c.Assert(err, IsNil)
		c.Assert(se[0].PrettyPrint(), Equals, ev.PrettyPrint())

		ct := "application/vnd.eventstore.events+json"
		contentType := r.Header.Get("Content-Type")
		c.Assert(ct, Equals, contentType)

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "")
	})

	streamWriter := client.NewStreamWriter(streamName)
	err := streamWriter.Append(nil, ev)

	c.Assert(err, IsNil)
}

func (s *StreamWriterSuite) TestAppendEventsMultiple(c *C) {
	et := "SomeEventType"
	d1 := &MyDataType{Field1: 445, Field2: "Some string"}
	d2 := &MyDataType{Field1: 446, Field2: "Some other string"}
	ev1 := goes.NewEvent("", et, d1, nil)
	ev2 := goes.NewEvent("", et, d2, nil)

	stream := "Some-Stream"
	url := fmt.Sprintf("/streams/%s", stream)

	mux.HandleFunc(url, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, "POST")

		b, _ := ioutil.ReadAll(r.Body)
		se := []goes.Event{}
		err := json.NewDecoder(bytes.NewReader(b)).Decode(&se)
		c.Assert(err, IsNil)
		c.Assert(se[0].PrettyPrint(), Equals, ev1.PrettyPrint())
		c.Assert(se[1].PrettyPrint(), Equals, ev2.PrettyPrint())

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "")
	})

	streamWriter := client.NewStreamWriter(stream)
	err := streamWriter.Append(nil, ev1, ev2)

	c.Assert(err, IsNil)
}

func (s *StreamWriterSuite) TestAppendEventsWithErrConcurrencyViolation(c *C) {
	data := &MyDataType{Field1: 445, Field2: "Some string"}
	et := "SomeEventType"
	ev := goes.NewEvent("", et, data, nil)

	stream := "Some-Stream"
	url := fmt.Sprintf("/streams/%s", stream)

	expectedVersion := 5

	mux.HandleFunc(url, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodPost)

		want := strconv.Itoa(expectedVersion)
		got := r.Header.Get("ES-ExpectedVersion")
		c.Assert(got, Equals, want)

		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "")

	})

	streamWriter := client.NewStreamWriter(stream)
	err := streamWriter.Append(&expectedVersion, ev)
	c.Assert(err, NotNil)
	c.Assert(reflect.TypeOf(err).Elem().Name(), DeepEquals, "ErrConcurrencyViolation")
}

func (s *StreamWriterSuite) TestAppendStreamMetadata(c *C) {
	eventType := "MetaData"
	stream := "SomeStream"

	// Before updating the metadata, the method needs to get the MetaData url
	// According to the docs, the eventstore team reserve the right to change
	// the metadata url.
	path := fmt.Sprintf("/streams/%s/0/forward/1", stream)
	fullURL := fmt.Sprintf("%s%s", server.URL, path)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		es := mock.CreateTestEvents(1, stream, server.URL, eventType)
		f, _ := mock.CreateTestFeed(es, fullURL)
		fmt.Fprint(w, f.PrettyPrint())
	})

	meta := fmt.Sprintf("{\"baz\":\"boo\"}")
	want := json.RawMessage(meta)

	url := fmt.Sprintf("/streams/%s/metadata", stream)

	mux.HandleFunc(url, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodPost)

		var got json.RawMessage
		ev := &goes.Event{Data: &got}
		err := json.NewDecoder(r.Body).Decode(ev)
		c.Assert(err, IsNil)
		c.Assert(got, DeepEquals, want)

		mt := "application/vnd.eventstore.events+json"
		mediaType := r.Header.Get("Content-Type")
		c.Assert(mt, Equals, mediaType)

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "")
	})

	writer := client.NewStreamWriter(stream)
	err := writer.WriteMetaData(stream, &want)
	c.Assert(err, IsNil)
}

func (s *StreamWriterSuite) TestAppendStreamMetadataReturnsUnauthorisedWhenGettingMetaURL(c *C) {
	stream := "SomeStream"
	path := fmt.Sprintf("/streams/%s/0/forward/1", stream)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "")
	})

	meta := fmt.Sprintf("{\"baz\":\"boo\"}")
	want := json.RawMessage(meta)

	writer := client.NewStreamWriter(stream)
	err := writer.WriteMetaData(stream, &want)

	c.Assert(err, NotNil)
	if e, ok := err.(*goes.ErrUnauthorized); ok {
		c.Assert(e.ErrorResponse, NotNil)
	} else {
		c.Error("Error returned is not of type *ErrUnauthorized")
	}
}

func (s *StreamWriterSuite) TestAppendStreamMetadataReturnsTemporarilyUnavailableWhenGettingMetaURL(c *C) {
	stream := "SomeStream"
	path := fmt.Sprintf("/streams/%s/0/forward/1", stream)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "")
	})

	meta := fmt.Sprintf("{\"baz\":\"boo\"}")
	want := json.RawMessage(meta)

	writer := client.NewStreamWriter(stream)
	err := writer.WriteMetaData(stream, &want)

	c.Assert(err, NotNil)
	if e, ok := err.(*goes.ErrTemporarilyUnavailable); ok {
		c.Assert(e.ErrorResponse, NotNil)
	} else {
		c.Error("Error returned is not of type *ErrTemporarilyUnavailable")
	}
}

func (s *StreamWriterSuite) TestAppendStreamMetadataReturnsErrUnexpectedWhenGettingMetaURL(c *C) {
	stream := "SomeStream"
	path := fmt.Sprintf("/streams/%s/0/forward/1", stream)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusHTTPVersionNotSupported)
		fmt.Fprint(w, "")
	})

	meta := fmt.Sprintf("{\"baz\":\"boo\"}")
	want := json.RawMessage(meta)

	writer := client.NewStreamWriter(stream)
	err := writer.WriteMetaData(stream, &want)

	c.Assert(err, NotNil)
	if e, ok := err.(*goes.ErrUnexpected); ok {
		c.Assert(e.ErrorResponse, NotNil)
	} else {
		c.Error("Error returned is not of type *ErrUnexpected")
	}
}

func (s *StreamWriterSuite) TestAppendStreamMetadataReturnsErrUnexpectedWhenWritingMeta(c *C) {
	eventType := "MetaData"
	stream := "SomeStream"

	// Before updating the metadata, the method needs to get the MetaData url
	// According to the docs, the eventstore team reserve the right to change
	// the metadata url.
	path := fmt.Sprintf("/streams/%s/0/forward/1", stream)
	fullURL := fmt.Sprintf("%s%s", server.URL, path)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		es := mock.CreateTestEvents(1, stream, server.URL, eventType)
		f, _ := mock.CreateTestFeed(es, fullURL)
		fmt.Fprint(w, f.PrettyPrint())
	})

	meta := fmt.Sprintf("{\"baz\":\"boo\"}")
	want := json.RawMessage(meta)

	url := fmt.Sprintf("/streams/%s/metadata", stream)

	mux.HandleFunc(url, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodPost)
		w.WriteHeader(http.StatusLengthRequired)
		fmt.Fprint(w, "")
	})

	writer := client.NewStreamWriter(stream)
	err := writer.WriteMetaData(stream, &want)

	c.Assert(err, NotNil)
	if e, ok := err.(*goes.ErrUnexpected); ok {
		c.Assert(e.ErrorResponse, NotNil)
	} else {
		c.Error("Error returned is not of type *ErrUnexpected")
	}
}

func (s *StreamWriterSuite) TestAppendStreamMetadataReturnsErrUnauthorizedWhenWritingMeta(c *C) {
	eventType := "MetaData"
	stream := "SomeStream"

	// Before updating the metadata, the method needs to get the MetaData url
	// According to the docs, the eventstore team reserve the right to change
	// the metadata url.
	path := fmt.Sprintf("/streams/%s/0/forward/1", stream)
	fullURL := fmt.Sprintf("%s%s", server.URL, path)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		es := mock.CreateTestEvents(1, stream, server.URL, eventType)
		f, _ := mock.CreateTestFeed(es, fullURL)
		fmt.Fprint(w, f.PrettyPrint())
	})

	meta := fmt.Sprintf("{\"baz\":\"boo\"}")
	want := json.RawMessage(meta)

	url := fmt.Sprintf("/streams/%s/metadata", stream)

	mux.HandleFunc(url, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodPost)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "")
	})

	writer := client.NewStreamWriter(stream)
	err := writer.WriteMetaData(stream, &want)

	c.Assert(err, NotNil)
	if e, ok := err.(*goes.ErrUnauthorized); ok {
		c.Assert(e.ErrorResponse, NotNil)
	} else {
		c.Error("Error returned is not of type *ErrUnauthorized")
	}
}

func (s *StreamWriterSuite) TestAppendStreamMetadataReturnsErrTemporarilyUnavailableWhenWritingMeta(c *C) {
	eventType := "MetaData"
	stream := "SomeStream"

	// Before updating the metadata, the method needs to get the MetaData url
	// According to the docs, the eventstore team reserve the right to change
	// the metadata url.
	path := fmt.Sprintf("/streams/%s/0/forward/1", stream)
	fullURL := fmt.Sprintf("%s%s", server.URL, path)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		es := mock.CreateTestEvents(1, stream, server.URL, eventType)
		f, _ := mock.CreateTestFeed(es, fullURL)
		fmt.Fprint(w, f.PrettyPrint())
	})

	meta := fmt.Sprintf("{\"baz\":\"boo\"}")
	want := json.RawMessage(meta)

	url := fmt.Sprintf("/streams/%s/metadata", stream)

	mux.HandleFunc(url, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodPost)
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "")
	})

	writer := client.NewStreamWriter(stream)
	err := writer.WriteMetaData(stream, &want)

	c.Assert(err, NotNil)
	if e, ok := err.(*goes.ErrTemporarilyUnavailable); ok {
		c.Assert(e.ErrorResponse, NotNil)
	} else {
		c.Error("Error returned is not of type *ErrTemporarilyUnavailable")
	}
}
