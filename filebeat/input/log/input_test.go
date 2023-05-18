// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

//go:build !integration
// +build !integration

package log

import (
	"io/ioutil"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/v7/filebeat/channel"
	"github.com/elastic/beats/v7/filebeat/input"
	"github.com/elastic/beats/v7/filebeat/input/file"
	"github.com/elastic/beats/v7/filebeat/input/inputtest"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/common/match"
	"github.com/elastic/beats/v7/libbeat/tests/resources"
)

func TestInputFileExclude(t *testing.T) {
	p := Input{
		config: config{
			ExcludeFiles: []match.Matcher{match.MustCompile(`\.gz$`)},
		},
	}

	assert.True(t, p.isFileExcluded("/tmp/log/logw.gz"))
	assert.False(t, p.isFileExcluded("/tmp/log/logw.log"))
}

var cleanInactiveTests = []struct {
	cleanInactive time.Duration
	fileTime      time.Time
	result        bool
}{
	{
		cleanInactive: 0,
		fileTime:      time.Now(),
		result:        false,
	},
	{
		cleanInactive: 1 * time.Second,
		fileTime:      time.Now().Add(-5 * time.Second),
		result:        true,
	},
	{
		cleanInactive: 10 * time.Second,
		fileTime:      time.Now().Add(-5 * time.Second),
		result:        false,
	},
}

func TestIsCleanInactive(t *testing.T) {
	for _, test := range cleanInactiveTests {

		l := Input{
			config: config{
				CleanInactive: test.cleanInactive,
			},
		}
		state := file.State{
			Fileinfo: TestFileInfo{
				time: test.fileTime,
			},
		}

		assert.Equal(t, test.result, l.isCleanInactive(state))
	}
}

func TestInputLifecycle(t *testing.T) {
	cases := []struct {
		title  string
		closer func(input.Context, *Input)
	}{
		{
			title: "explicitly closed",
			closer: func(_ input.Context, input *Input) {
				input.Wait()
			},
		},
		{
			title: "context done",
			closer: func(ctx input.Context, _ *Input) {
				close(ctx.Done)
			},
		},
		{
			title: "beat context done",
			closer: func(ctx input.Context, _ *Input) {
				close(ctx.Done)
				close(ctx.BeatDone)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			context := input.Context{
				Done:     make(chan struct{}),
				BeatDone: make(chan struct{}),
			}
			testInputLifecycle(t, context, c.closer)
		})
	}
}

// TestInputLifecycle performs blackbock testing of the log input
func testInputLifecycle(t *testing.T, context input.Context, closer func(input.Context, *Input)) {
	goroutines := resources.NewGoroutinesChecker()
	defer goroutines.Check(t)

	// Prepare a log file
	tmpdir, err := ioutil.TempDir(os.TempDir(), "input-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	logs := []byte("some log line\nother log line\n")
	err = ioutil.WriteFile(path.Join(tmpdir, "some.log"), logs, 0644)
	assert.NoError(t, err)

	// Setup the input
	config, _ := common.NewConfigFrom(common.MapStr{
		"paths":     path.Join(tmpdir, "*.log"),
		"close_eof": true,
	})

	events := make(chan beat.Event, 100)
	defer close(events)
	capturer := NewEventCapturer(events)
	defer capturer.Close()
	connector := channel.ConnectorFunc(func(_ *common.Config, _ beat.ClientConfig) (channel.Outleter, error) {
		return channel.SubOutlet(capturer), nil
	})

	input, err := NewInput(config, connector, context)
	if err != nil {
		t.Error(err)
		return
	}

	// Run the input and wait for finalization
	input.Run()

	timeout := time.After(30 * time.Second)
	done := make(chan struct{})
	for {
		select {
		case event := <-events:
			if state, ok := event.Private.(file.State); ok && state.Finished {
				assert.Equal(t, len(logs), int(state.Offset), "file has not been fully read")
				go func() {
					closer(context, input.(*Input))
					close(done)
				}()
			}
		case <-done:
			return
		case <-timeout:
			t.Fatal("timeout waiting for closed state")
		}
	}
}

func TestNewInputDone(t *testing.T) {
	config := common.MapStr{
		"paths": path.Join(os.TempDir(), "logs", "*.log"),
	}
	inputtest.AssertNotStartedInputCanBeDone(t, NewInput, &config)
}

func TestNewInputError(t *testing.T) {
	goroutines := resources.NewGoroutinesChecker()
	defer goroutines.Check(t)

	config := common.NewConfig()

	connector := channel.ConnectorFunc(func(_ *common.Config, _ beat.ClientConfig) (channel.Outleter, error) {
		return inputtest.Outlet{}, nil
	})

	context := input.Context{}

	_, err := NewInput(config, connector, context)
	assert.Error(t, err)
}

func TestMatchesMeta(t *testing.T) {
	tests := []struct {
		Input  *Input
		Meta   map[string]string
		Result bool
	}{
		{
			Input: &Input{
				meta: map[string]string{
					"it": "matches",
				},
			},
			Meta: map[string]string{
				"it": "matches",
			},
			Result: true,
		},
		{
			Input: &Input{
				meta: map[string]string{
					"it":     "doesnt",
					"doesnt": "match",
				},
			},
			Meta: map[string]string{
				"it": "doesnt",
			},
			Result: false,
		},
		{
			Input: &Input{
				meta: map[string]string{
					"it": "doesnt",
				},
			},
			Meta: map[string]string{
				"it":     "doesnt",
				"doesnt": "match",
			},
			Result: false,
		},
		{
			Input: &Input{
				meta: map[string]string{},
			},
			Meta:   map[string]string{},
			Result: true,
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.Result, test.Input.matchesMeta(test.Meta))
	}
}

func TestMatchPath(t *testing.T) {
	inputs := []*Input{
		&Input{
			config: config{
				Paths: []string{
					"/var/log/containers/*0a5e622c9d1fbbe6557a6ee6d4d036996d284c50ea5b31bc4c40cc9c27814e7c.log",
				},
			},
		},
	}
	states := []*file.State{
		&file.State{Source: "/var/log/containers/b4b2693a17be4cf285f4b686561cbbef-cronjob-7ltv4-28052760-8whmb_tekton-pipelines_script-0a5e622c9d1fbbe6557a6ee6d4d036996d284c50ea5b31bc4c40cc9c27814e7c.log"},
		&file.State{Source: "/var/log/containers/68d0f86c40fd4d68b47835a270f42457-manual-xq799-3vg18nz60dft-pod_tekton-pipelines_step-sca-scan-616d0f223867a4e3f33b773882b2154fd1f9410661cf35d5704e8ab9d2a8deb7.log"},
		&file.State{Source: "/var/log/containers/e86e5fd0e7d243a7b509ee540f3f5b4a-manual-98cw4-27kpluvv8mm1-pod_tekton-pipelines_prepare-3579afaf98f44f1aa4df776b1a642040f847f69902944f3a11b824c759d78278.log"},
		&file.State{Source: "/var/log/containers/prometheus-monitoring-kube-prometheus-prometheus-0_monitoring_prometheus-a95d89f2a4f070dfb19d45b3c2513d3f03333eaccb03007c996816150cf2c566.log"},
		&file.State{Source: "/var/log/containers/644fff3e902f424c80584824e1728ecf-manual-rh9b2-hk21ftuqxc18-pod_tekton-pipelines_prepare-3b3c0470f37a6e2e2a4daf2edabdd330e6e1671131c903a235ce3afcd44e17de.log"},
		&file.State{Source: "/var/log/containers/81bccede021f4dfcbe62fb0147e47c9c-manual-vhlds-6wulb5wiihn3-pod_tekton-pipelines_step-codesec-scan-f8c2dfbde648f81ccbe4311444c5e7e61490e301f1895c6e0f563d23ff15a3de.log"},
		&file.State{Source: "/var/log/containers/73c5413e5aba4d4fae94719335bb1735-manual-nwxx6-v03pvt6514e1-pod_tekton-pipelines_step-clone-bddfe72431d190239b8ce2bb6b64eff159197bdc19b4f67e5a124b7a4401c55a.log"},
	}
	for _, input := range inputs {
		for _, state := range states {
			input.matchesFile(state.Source)
		}
	}
}

func BenchmarkMatchPath(b *testing.B) {
	inputs := []*Input{
		&Input{
			config: config{
				Paths: []string{
					"/var/log/containers/*0a5e622c9d1fbbe6557a6ee6d4d036996d284c50ea5b31bc4c40cc9c27814e7c.log",
				},
			},
		},
	}
	states := []*file.State{
		&file.State{Source: "/var/log/containers/b4b2693a17be4cf285f4b686561cbbef-cronjob-7ltv4-28052760-8whmb_tekton-pipelines_script-0a5e622c9d1fbbe6557a6ee6d4d036996d284c50ea5b31bc4c40cc9c27814e7c.log"},
		&file.State{Source: "/var/log/containers/68d0f86c40fd4d68b47835a270f42457-manual-xq799-3vg18nz60dft-pod_tekton-pipelines_step-sca-scan-616d0f223867a4e3f33b773882b2154fd1f9410661cf35d5704e8ab9d2a8deb7.log"},
		&file.State{Source: "/var/log/containers/e86e5fd0e7d243a7b509ee540f3f5b4a-manual-98cw4-27kpluvv8mm1-pod_tekton-pipelines_prepare-3579afaf98f44f1aa4df776b1a642040f847f69902944f3a11b824c759d78278.log"},
		&file.State{Source: "/var/log/containers/prometheus-monitoring-kube-prometheus-prometheus-0_monitoring_prometheus-a95d89f2a4f070dfb19d45b3c2513d3f03333eaccb03007c996816150cf2c566.log"},
		&file.State{Source: "/var/log/containers/644fff3e902f424c80584824e1728ecf-manual-rh9b2-hk21ftuqxc18-pod_tekton-pipelines_prepare-3b3c0470f37a6e2e2a4daf2edabdd330e6e1671131c903a235ce3afcd44e17de.log"},
		&file.State{Source: "/var/log/containers/81bccede021f4dfcbe62fb0147e47c9c-manual-vhlds-6wulb5wiihn3-pod_tekton-pipelines_step-codesec-scan-f8c2dfbde648f81ccbe4311444c5e7e61490e301f1895c6e0f563d23ff15a3de.log"},
		&file.State{Source: "/var/log/containers/73c5413e5aba4d4fae94719335bb1735-manual-nwxx6-v03pvt6514e1-pod_tekton-pipelines_step-clone-bddfe72431d190239b8ce2bb6b64eff159197bdc19b4f67e5a124b7a4401c55a.log"},
	}
	b.ResetTimer()
	for _, input := range inputs {
		for _, state := range states {
			input.matchesFile(state.Source)
		}
	}
}

type TestFileInfo struct {
	time time.Time
}

func (t TestFileInfo) Name() string       { return "" }
func (t TestFileInfo) Size() int64        { return 0 }
func (t TestFileInfo) Mode() os.FileMode  { return 0 }
func (t TestFileInfo) ModTime() time.Time { return t.time }
func (t TestFileInfo) IsDir() bool        { return false }
func (t TestFileInfo) Sys() interface{}   { return nil }

type eventCapturer struct {
	closed    bool
	c         chan struct{}
	closeOnce sync.Once
	events    chan beat.Event
}

func NewEventCapturer(events chan beat.Event) channel.Outleter {
	return &eventCapturer{
		c:      make(chan struct{}),
		events: events,
	}
}

func (o *eventCapturer) OnEvent(event beat.Event) bool {
	o.events <- event
	return true
}

func (o *eventCapturer) Close() error {
	o.closeOnce.Do(func() {
		o.closed = true
		close(o.c)
	})
	return nil
}

func (o *eventCapturer) Done() <-chan struct{} {
	return o.c
}
