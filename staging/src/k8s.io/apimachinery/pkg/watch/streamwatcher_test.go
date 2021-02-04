/*
Copyright 2014 The Kubernetes Authors.

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

package watch_test

import (
	"fmt"
	"io"
	"reflect"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	. "k8s.io/apimachinery/pkg/watch"
)

type fakeDecoder struct {
	lock   sync.Mutex
	items  chan Event
	err    error
	count  int
	closed bool
}

func (f *fakeDecoder) getErr() error {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.err
}

func (f *fakeDecoder) setErr(err error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.err = err
}

func (f *fakeDecoder) getCount() int {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.count
}

func (f *fakeDecoder) Decode() (action EventType, object runtime.Object, err error) {
	err = f.getErr()
	if err != nil {
		f.lock.Lock()
		defer f.lock.Unlock()
		f.count++
		return "", nil, err
	}
	item, open := <-f.items
	if !open {
		err = f.getErr()
		if err != nil {
			return "", nil, err
		}
		return action, nil, io.EOF
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	f.count++
	return item.Type, item.Object, nil
}

func (f *fakeDecoder) Close() {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.items != nil && !f.closed {
		f.closed = true
		close(f.items)
	}
}

type fakeReporter struct {
	err error
}

func (f *fakeReporter) AsObject(err error) runtime.Object {
	f.err = err
	return runtime.Unstructured(nil)
}

func TestStreamWatcher(t *testing.T) {
	table := []Event{
		{Type: Added, Object: testType("foo")},
	}

	fd := &fakeDecoder{items: make(chan Event, 5)}
	sw := NewStreamWatcher(fd, nil)

	for _, item := range table {
		fd.items <- item
		got, open := <-sw.ResultChan()
		if !open {
			t.Errorf("unexpected early close")
		}
		if e, a := item, got; !reflect.DeepEqual(e, a) {
			t.Errorf("expected %v, got %v", e, a)
		}
	}

	sw.Stop()
	_, open := <-sw.ResultChan()
	if open {
		t.Errorf("Unexpected failure to close")
	}
}

func TestStreamWatcherError(t *testing.T) {
	fd := &fakeDecoder{err: fmt.Errorf("test error")}
	fr := &fakeReporter{}
	sw := NewStreamWatcher(fd, fr)
	evt, ok := <-sw.ResultChan()
	if !ok {
		t.Fatalf("unexpected close")
	}
	if evt.Type != Error || evt.Object != runtime.Unstructured(nil) {
		t.Fatalf("unexpected object: %#v", evt)
	}
	_, ok = <-sw.ResultChan()
	if ok {
		t.Fatalf("unexpected open channel")
	}

	sw.Stop()
	_, ok = <-sw.ResultChan()
	if ok {
		t.Fatalf("unexpected open channel")
	}
}

func TestStreamWatcherStopRace(t *testing.T) {
	table := []Event{
		{Type: Added, Object: testType("foo")},
		{Type: Added, Object: testType("bar")},
	}

	fd := &fakeDecoder{items: make(chan Event, 5)}
	fr := &fakeReporter{}
	sw := NewStreamWatcher(fd, fr)

	for i, item := range table {
		fd.items <- item
		t.Logf("sent event %d: %+v\n", i+1, item)
	}

	t.Logf("waiting for event 1\n")
	got, open := <-sw.ResultChan()
	if !open {
		t.Errorf("unexpected early close")
	}
	if e, a := table[0], got; !reflect.DeepEqual(e, a) {
		t.Errorf("expected %v, got %v", e, a)
	} else {
		t.Logf("GOT 1: %+v\n", got)
	}

	for fd.getCount() < 2 {
		t.Logf("waiting for receive...\n")
		goruntime.Gosched()
	}
	t.Logf("waiting for event 2\n")
	got, open = <-sw.ResultChan()
	if !open {
		t.Errorf("unexpected early close")
	}
	if e, a := table[1], got; !reflect.DeepEqual(e, a) {
		t.Errorf("expected %v, got %v", e, a)
	} else {
		t.Logf("GOT 2: %+v\n", got)
	}

	fd.setErr(fmt.Errorf("some stop error on underlying watch stream"))
	sw.Test() // enforce a stop just after the stop check in stream error handling
	fd.Close()
	// Be sure the receive go routine had a chance to run.
	// (and react on the new stop channel)
	goruntime.Gosched()
	count := 0
	for sw.StopCount() < 2 {
		count++
		if count > 40 {
			t.Errorf("method receive did not stop after 40 retries")
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}
