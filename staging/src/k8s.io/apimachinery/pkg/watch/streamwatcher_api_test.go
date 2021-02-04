/*
Copyright 2021 The Kubernetes Authors.

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

package watch

// Test initiates a test
func (sw *StreamWatcher) Test() {
	sw.Lock()
	defer sw.Unlock()
	sw.test = sw._testing
}

// StopCount reports number of stops
func (sw *StreamWatcher) StopCount() int {
	sw.Lock()
	defer sw.Unlock()
	return sw.stopcount
}

// testing internally stops in test mode
func (sw *StreamWatcher) _testing() {
	if sw.stopcount == 0 {
		close(sw.done)
		sw.source.Close()
	}
	sw.stopcount++
}
