/*
Copyright 2024 The Flux authors

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

package v2

import (
	"reflect"
	"testing"
)

func TestSnapshots_Sort(t *testing.T) {
	tests := []struct {
		name string
		in   Snapshots
		want Snapshots
	}{
		{
			name: "sorts by descending version",
			in: Snapshots{
				{Version: 1},
				{Version: 3},
				{Version: 2},
			},
			want: Snapshots{
				{Version: 3},
				{Version: 2},
				{Version: 1},
			},
		},
		{
			name: "already sorted",
			in: Snapshots{
				{Version: 3},
				{Version: 2},
				{Version: 1},
			},
			want: Snapshots{
				{Version: 3},
				{Version: 2},
				{Version: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.SortByVersion()

			if !reflect.DeepEqual(tt.in, tt.want) {
				t.Errorf("SortByVersion() got %v, want %v", tt.in, tt.want)
			}
		})
	}
}

func TestSnapshots_Latest(t *testing.T) {
	tests := []struct {
		name string
		in   Snapshots
		want *Snapshot
	}{
		{
			name: "returns most recent snapshot",
			in: Snapshots{
				{Version: 1},
				{Version: 3},
				{Version: 2},
			},
			want: &Snapshot{Version: 3},
		},
		{
			name: "returns nil if empty",
			in:   Snapshots{},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Latest(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Latest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSnapshots_Previous(t *testing.T) {
	tests := []struct {
		name        string
		in          Snapshots
		ignoreTests bool
		want        *Snapshot
	}{
		{
			name: "returns previous snapshot",
			in: Snapshots{
				{Version: 2, Status: "deployed"},
				{Version: 3, Status: "failed"},
				{Version: 1, Status: "superseded"},
			},
			want: &Snapshot{Version: 2, Status: "deployed"},
		},
		{
			name: "includes snapshots with failed tests",
			in: Snapshots{
				{Version: 4, Status: "deployed"},
				{Version: 1, Status: "superseded"},
				{Version: 2, Status: "superseded"},
				{Version: 3, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"test": {Phase: "Failed"},
				}},
			},
			ignoreTests: true,
			want: &Snapshot{Version: 3, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
				"test": {Phase: "Failed"},
			}},
		},
		{
			name: "ignores snapshots with failed tests",
			in: Snapshots{
				{Version: 4, Status: "deployed"},
				{Version: 1, Status: "superseded"},
				{Version: 2, Status: "superseded"},
				{Version: 3, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"test": {Phase: "Failed"},
				}},
			},
			ignoreTests: false,
			want:        &Snapshot{Version: 2, Status: "superseded"},
		},
		{
			name: "returns nil without previous snapshot",
			in: Snapshots{
				{Version: 1, Status: "deployed"},
			},
			want: nil,
		},
		{
			name: "returns nil without snapshot matching criteria",
			in: Snapshots{
				{Version: 4, Status: "deployed"},
				{Version: 3, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"test": {Phase: "Failed"},
				}},
			},
			ignoreTests: false,
			want:        nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Previous(tt.ignoreTests); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Previous() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSnapshots_Truncate(t *testing.T) {
	tests := []struct {
		name        string
		in          Snapshots
		ignoreTests bool
		want        Snapshots
	}{
		{
			name: "keeps previous snapshot",
			in: Snapshots{
				{Version: 1, Status: "superseded"},
				{Version: 3, Status: "failed"},
				{Version: 2, Status: "superseded"},
				{Version: 4, Status: "deployed"},
			},
			want: Snapshots{
				{Version: 4, Status: "deployed"},
				{Version: 3, Status: "failed"},
				{Version: 2, Status: "superseded"},
			},
		},
		{
			name: "ignores snapshots with failed tests",
			in: Snapshots{
				{Version: 4, Status: "deployed"},
				{Version: 3, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"upgrade-test-fail-podinfo-fault-test-tiz9x": {Phase: "Failed"},
					"upgrade-test-fail-podinfo-grpc-test-gddcw":  {},
				}},
				{Version: 2, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"upgrade-test-fail-podinfo-grpc-test-h0tc2": {
						Phase: "Succeeded",
					},
					"upgrade-test-fail-podinfo-jwt-test-vzusa": {
						Phase: "Succeeded",
					},
					"upgrade-test-fail-podinfo-service-test-b647e": {
						Phase: "Succeeded",
					},
				}},
			},
			ignoreTests: false,
			want: Snapshots{
				{Version: 4, Status: "deployed"},
				{Version: 3, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"upgrade-test-fail-podinfo-fault-test-tiz9x": {Phase: "Failed"},
					"upgrade-test-fail-podinfo-grpc-test-gddcw":  {},
				}},
				{Version: 2, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"upgrade-test-fail-podinfo-grpc-test-h0tc2": {
						Phase: "Succeeded",
					},
					"upgrade-test-fail-podinfo-jwt-test-vzusa": {
						Phase: "Succeeded",
					},
					"upgrade-test-fail-podinfo-service-test-b647e": {
						Phase: "Succeeded",
					},
				}},
			},
		},
		{
			name: "keeps previous snapshot with failed tests",
			in: Snapshots{
				{Version: 4, Status: "deployed"},
				{Version: 3, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"upgrade-test-fail-podinfo-fault-test-tiz9x": {Phase: "Failed"},
					"upgrade-test-fail-podinfo-grpc-test-gddcw":  {},
				}},
				{Version: 2, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"upgrade-test-fail-podinfo-grpc-test-h0tc2": {
						Phase: "Succeeded",
					},
					"upgrade-test-fail-podinfo-jwt-test-vzusa": {
						Phase: "Succeeded",
					},
					"upgrade-test-fail-podinfo-service-test-b647e": {
						Phase: "Succeeded",
					},
				}},
				{Version: 1, Status: "superseded"},
			},
			ignoreTests: true,
			want: Snapshots{
				{Version: 4, Status: "deployed"},
				{Version: 3, Status: "superseded", TestHooks: &map[string]*TestHookStatus{
					"upgrade-test-fail-podinfo-fault-test-tiz9x": {Phase: "Failed"},
					"upgrade-test-fail-podinfo-grpc-test-gddcw":  {},
				}},
			},
		},
		{
			name: "retains most recent snapshots when all have failed",
			in: Snapshots{
				{Version: 6, Status: "deployed"},
				{Version: 5, Status: "failed"},
				{Version: 4, Status: "failed"},
				{Version: 3, Status: "failed"},
				{Version: 2, Status: "failed"},
				{Version: 1, Status: "failed"},
			},
			want: Snapshots{
				{Version: 6, Status: "deployed"},
				{Version: 5, Status: "failed"},
				{Version: 4, Status: "failed"},
				{Version: 3, Status: "failed"},
				{Version: 2, Status: "failed"},
			},
		},
		{
			name: "without previous snapshot",
			in: Snapshots{
				{Version: 1, Status: "deployed"},
			},
			want: Snapshots{
				{Version: 1, Status: "deployed"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Truncate(tt.ignoreTests)

			if !reflect.DeepEqual(tt.in, tt.want) {
				t.Errorf("Truncate() got %v, want %v", tt.in, tt.want)
			}
		})
	}
}
