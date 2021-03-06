// Copyright 2020 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package build

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"android/soong/ui/logger"
)

func TestPruneMetricsFiles(t *testing.T) {
	rootDir := t.TempDir()

	dirs := []string{
		filepath.Join(rootDir, "d1"),
		filepath.Join(rootDir, "d1", "d2"),
		filepath.Join(rootDir, "d1", "d2", "d3"),
	}

	files := []string{
		filepath.Join(rootDir, "d1", "f1"),
		filepath.Join(rootDir, "d1", "d2", "f1"),
		filepath.Join(rootDir, "d1", "d2", "d3", "f1"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0777); err != nil {
			t.Fatalf("got %v, expecting nil error for making directory %q", err, d)
		}
	}

	for _, f := range files {
		if err := ioutil.WriteFile(f, []byte{}, 0777); err != nil {
			t.Fatalf("got %v, expecting nil error on writing file %q", err, f)
		}
	}

	want := []string{
		filepath.Join(rootDir, "d1", "f1"),
		filepath.Join(rootDir, "d1", "d2", "f1"),
		filepath.Join(rootDir, "d1", "d2", "d3", "f1"),
	}

	got := pruneMetricsFiles([]string{rootDir})

	sort.Strings(got)
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q after pruning metrics files", got, want)
	}
}

func TestUploadMetrics(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		description string
		uploader    string
		createFiles bool
		files       []string
	}{{
		description: "no metrics uploader",
	}, {
		description: "non-existent metrics files no upload",
		uploader:    "echo",
		files:       []string{"metrics_file_1", "metrics_file_2", "metrics_file_3"},
	}, {
		description: "trigger upload",
		uploader:    "echo",
		createFiles: true,
		files:       []string{"metrics_file_1", "metrics_file_2"},
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer logger.Recover(func(err error) {
				t.Fatalf("got unexpected error: %v", err)
			})

			outDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create out directory: %v", outDir)
			}
			defer os.RemoveAll(outDir)

			// Supply our own tmpDir to delete the temp dir once the test is done.
			orgTmpDir := tmpDir
			tmpDir = func(string, string) (string, error) {
				retDir := filepath.Join(outDir, "tmp_upload_dir")
				if err := os.Mkdir(retDir, 0755); err != nil {
					t.Fatalf("failed to create temporary directory %q: %v", retDir, err)
				}
				return retDir, nil
			}
			defer func() { tmpDir = orgTmpDir }()

			metricsUploadDir := filepath.Join(outDir, ".metrics_uploader")
			if err := os.Mkdir(metricsUploadDir, 0755); err != nil {
				t.Fatalf("failed to create %q directory for oauth valid check: %v", metricsUploadDir, err)
			}

			var metricsFiles []string
			if tt.createFiles {
				for _, f := range tt.files {
					filename := filepath.Join(outDir, f)
					metricsFiles = append(metricsFiles, filename)
					if err := ioutil.WriteFile(filename, []byte("test file"), 0644); err != nil {
						t.Fatalf("failed to create a fake metrics file %q for uploading: %v", filename, err)
					}
				}
			}

			config := Config{&configImpl{
				environ: &Environment{
					"OUT_DIR=" + outDir,
				},
				buildDateTime:   strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10),
				metricsUploader: tt.uploader,
			}}

			UploadMetrics(ctx, config, false, time.Now(), metricsFiles...)
		})
	}
}

func TestUploadMetricsErrors(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		description string
		tmpDir      string
		tmpDirErr   error
		expectedErr string
	}{{
		description: "getTmpDir returned error",
		tmpDirErr:   errors.New("getTmpDir failed"),
		expectedErr: "getTmpDir failed",
	}, {
		description: "copyFile operation error",
		tmpDir:      "/fake_dir",
		expectedErr: "failed to copy",
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer logger.Recover(func(err error) {
				got := err.Error()
				if !strings.Contains(got, tt.expectedErr) {
					t.Errorf("got %q, want %q to be contained in error", got, tt.expectedErr)
				}
			})

			outDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create out directory: %v", outDir)
			}
			defer os.RemoveAll(outDir)

			orgTmpDir := tmpDir
			tmpDir = func(string, string) (string, error) {
				return tt.tmpDir, tt.tmpDirErr
			}
			defer func() { tmpDir = orgTmpDir }()

			metricsFile := filepath.Join(outDir, "metrics_file_1")
			if err := ioutil.WriteFile(metricsFile, []byte("test file"), 0644); err != nil {
				t.Fatalf("failed to create a fake metrics file %q for uploading: %v", metricsFile, err)
			}

			config := Config{&configImpl{
				environ: &Environment{
					"OUT_DIR=/bad",
				},
				metricsUploader: "echo",
			}}

			UploadMetrics(ctx, config, true, time.Now(), metricsFile)
			t.Errorf("got nil, expecting %q as a failure", tt.expectedErr)
		})
	}
}
