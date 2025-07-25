// Copyright © 2017 KhulnaSoft Security Software Ltd. <info@khulnasoft.com>
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

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/magiconair/properties/assert"

	"github.com/khulnasoft-lab/kube-bench/check"
	"github.com/spf13/viper"
)

var (
	g      string
	e      []error
	eIndex int
)

func fakeps(proc string) string {
	return g
}

func fakestat(file string) (os.FileInfo, error) {
	err := e[eIndex]
	eIndex++
	return nil, err
}

func TestVerifyBin(t *testing.T) {
	cases := []struct {
		proc  string
		psOut string
		exp   bool
	}{
		{proc: "single", psOut: "single", exp: true},
		{proc: "single", psOut: "", exp: false},
		{proc: "two words", psOut: "two words", exp: true},
		{proc: "two words", psOut: "", exp: false},
		{proc: "cmd", psOut: "cmd param1 param2", exp: true},
		{proc: "cmd param", psOut: "cmd param1 param2", exp: true},
		{proc: "cmd param", psOut: "cmd", exp: false},
		{proc: "cmd", psOut: "cmd x \ncmd y", exp: true},
		{proc: "cmd y", psOut: "cmd x \ncmd y", exp: true},
		{proc: "cmd", psOut: "/usr/bin/cmd", exp: true},
		{proc: "cmd", psOut: "kube-cmd", exp: false},
		{proc: "cmd", psOut: "/usr/bin/kube-cmd", exp: false},
	}

	psFunc = fakeps
	for id, c := range cases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			g = c.psOut
			v := verifyBin(c.proc)
			if v != c.exp {
				t.Fatalf("Expected %v got %v", c.exp, v)
			}
		})
	}
}

func TestFindExecutable(t *testing.T) {
	cases := []struct {
		candidates []string // list of executables we'd consider
		psOut      string   // fake output from ps
		exp        string   // the one we expect to find in the (fake) ps output
		expErr     bool
	}{
		{candidates: []string{"one", "two", "three"}, psOut: "two", exp: "two"},
		{candidates: []string{"one", "two", "three"}, psOut: "two three", exp: "two"},
		{candidates: []string{"one double", "two double", "three double"}, psOut: "two double is running", exp: "two double"},
		{candidates: []string{"one", "two", "three"}, psOut: "blah", expErr: true},
		{candidates: []string{"one double", "two double", "three double"}, psOut: "two", expErr: true},
		{candidates: []string{"apiserver", "kube-apiserver"}, psOut: "kube-apiserver", exp: "kube-apiserver"},
		{candidates: []string{"apiserver", "kube-apiserver", "hyperkube-apiserver"}, psOut: "kube-apiserver", exp: "kube-apiserver"},
	}

	psFunc = fakeps
	for id, c := range cases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			g = c.psOut
			e, err := findExecutable(c.candidates)
			if e != c.exp {
				t.Fatalf("Expected %v got %v", c.exp, e)
			}

			if err == nil && c.expErr {
				t.Fatalf("Expected error")
			}

			if err != nil && !c.expErr {
				t.Fatalf("Didn't expect error: %v", err)
			}
		})
	}
}

func TestGetBinaries(t *testing.T) {
	cases := []struct {
		config    map[string]interface{}
		psOut     string
		exp       map[string]string
		expectErr bool
	}{
		{
			config:    map[string]interface{}{"components": []string{"apiserver"}, "apiserver": map[string]interface{}{"bins": []string{"apiserver", "kube-apiserver"}}},
			psOut:     "kube-apiserver",
			exp:       map[string]string{"apiserver": "kube-apiserver"},
			expectErr: false,
		},
		{
			// "thing" is not in the list of components
			config:    map[string]interface{}{"components": []string{"apiserver"}, "apiserver": map[string]interface{}{"bins": []string{"apiserver", "kube-apiserver"}}, "thing": map[string]interface{}{"bins": []string{"something else", "thing"}}},
			psOut:     "kube-apiserver thing",
			exp:       map[string]string{"apiserver": "kube-apiserver"},
			expectErr: false,
		},
		{
			// "anotherthing" in list of components but doesn't have a definition
			config:    map[string]interface{}{"components": []string{"apiserver", "anotherthing"}, "apiserver": map[string]interface{}{"bins": []string{"apiserver", "kube-apiserver"}}, "thing": map[string]interface{}{"bins": []string{"something else", "thing"}}},
			psOut:     "kube-apiserver thing",
			exp:       map[string]string{"apiserver": "kube-apiserver"},
			expectErr: false,
		},
		{
			// more than one component
			config:    map[string]interface{}{"components": []string{"apiserver", "thing"}, "apiserver": map[string]interface{}{"bins": []string{"apiserver", "kube-apiserver"}}, "thing": map[string]interface{}{"bins": []string{"something else", "thing"}}},
			psOut:     "kube-apiserver \nthing",
			exp:       map[string]string{"apiserver": "kube-apiserver", "thing": "thing"},
			expectErr: false,
		},
		{
			// default binary to component name
			config:    map[string]interface{}{"components": []string{"apiserver", "thing"}, "apiserver": map[string]interface{}{"bins": []string{"apiserver", "kube-apiserver"}}, "thing": map[string]interface{}{"bins": []string{"something else", "thing"}, "optional": true}},
			psOut:     "kube-apiserver \notherthing some params",
			exp:       map[string]string{"apiserver": "kube-apiserver", "thing": "thing"},
			expectErr: false,
		},
		{
			// missing mandatory component
			config:    map[string]interface{}{"components": []string{"apiserver", "thing"}, "apiserver": map[string]interface{}{"bins": []string{"apiserver", "kube-apiserver"}}, "thing": map[string]interface{}{"bins": []string{"something else", "thing"}, "optional": true}},
			psOut:     "otherthing some params",
			exp:       map[string]string{"apiserver": "kube-apiserver", "thing": "thing"},
			expectErr: true,
		},
	}

	v := viper.New()
	psFunc = fakeps

	for id, c := range cases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			g = c.psOut
			for k, val := range c.config {
				v.Set(k, val)
			}
			m, err := getBinaries(v, check.MASTER)
			if c.expectErr {
				if err == nil {
					t.Fatal("Got nil Expected error")
				}
			} else if !reflect.DeepEqual(m, c.exp) {
				t.Fatalf("Got %v\nExpected %v", m, c.exp)
			}
		})
	}
}

func TestMultiWordReplace(t *testing.T) {
	cases := []struct {
		input   string
		sub     string
		subname string
		output  string
	}{
		{input: "Here's a file with no substitutions", sub: "blah", subname: "blah", output: "Here's a file with no substitutions"},
		{input: "Here's a file with a substitution", sub: "blah", subname: "substitution", output: "Here's a file with a blah"},
		{input: "Here's a file with multi-word substitutions", sub: "multi word", subname: "multi-word", output: "Here's a file with 'multi word' substitutions"},
		{input: "Here's a file with several several substitutions several", sub: "blah", subname: "several", output: "Here's a file with blah blah substitutions blah"},
	}
	for id, c := range cases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			s := multiWordReplace(c.input, c.subname, c.sub)
			if s != c.output {
				t.Fatalf("Expected %s got %s", c.output, s)
			}
		})
	}
}

func Test_getVersionFromKubectlOutput(t *testing.T) {
	ver := getVersionFromKubectlOutput(`{
  "serverVersion": {
    "major": "1",
    "minor": "8",
    "gitVersion": "v1.8.0"
  }
}`)
	if ver.BaseVersion() != "1.8" {
		t.Fatalf("Expected 1.8 got %s", ver.BaseVersion())
	}

	ver = getVersionFromKubectlOutput("Something completely different")
	if ver.BaseVersion() != defaultKubeVersion {
		t.Fatalf("Expected %s got %s", defaultKubeVersion, ver.BaseVersion())
	}
}

func TestFindConfigFile(t *testing.T) {
	cases := []struct {
		input       []string
		statResults []error
		exp         string
	}{
		{input: []string{"myfile"}, statResults: []error{nil}, exp: "myfile"},
		{input: []string{"thisfile", "thatfile"}, statResults: []error{os.ErrNotExist, nil}, exp: "thatfile"},
		{input: []string{"thisfile", "thatfile"}, statResults: []error{os.ErrNotExist, os.ErrNotExist}, exp: ""},
		{input: []string{"thisfile", "/etc/dummy/thatfile"}, statResults: []error{os.ErrNotExist, errors.New("stat /etc/dummy/thatfile: not a directory")}, exp: ""},
	}

	statFunc = fakestat
	for id, c := range cases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			e = c.statResults
			eIndex = 0
			conf := findConfigFile(c.input)
			if conf != c.exp {
				t.Fatalf("Got %s expected %s", conf, c.exp)
			}
		})
	}
}

func TestGetConfigFiles(t *testing.T) {
	cases := []struct {
		config      map[string]interface{}
		exp         map[string]string
		statResults []error
	}{
		{
			config:      map[string]interface{}{"components": []string{"apiserver"}, "apiserver": map[string]interface{}{"confs": []string{"apiserver", "kube-apiserver"}}},
			statResults: []error{os.ErrNotExist, nil},
			exp:         map[string]string{"apiserver": "kube-apiserver"},
		},
		{
			// Component "thing" isn't included in the list of components
			config: map[string]interface{}{
				"components": []string{"apiserver"},
				"apiserver":  map[string]interface{}{"confs": []string{"apiserver", "kube-apiserver"}},
				"thing":      map[string]interface{}{"confs": []string{"/my/file/thing"}},
			},
			statResults: []error{os.ErrNotExist, nil},
			exp:         map[string]string{"apiserver": "kube-apiserver"},
		},
		{
			// More than one component
			config: map[string]interface{}{
				"components": []string{"apiserver", "thing"},
				"apiserver":  map[string]interface{}{"confs": []string{"apiserver", "kube-apiserver"}},
				"thing":      map[string]interface{}{"confs": []string{"/my/file/thing"}},
			},
			statResults: []error{os.ErrNotExist, nil, nil},
			exp:         map[string]string{"apiserver": "kube-apiserver", "thing": "/my/file/thing"},
		},
		{
			// Default thing to specified default config
			config: map[string]interface{}{
				"components": []string{"apiserver", "thing"},
				"apiserver":  map[string]interface{}{"confs": []string{"apiserver", "kube-apiserver"}},
				"thing":      map[string]interface{}{"confs": []string{"/my/file/thing"}, "defaultconf": "another/thing"},
			},
			statResults: []error{os.ErrNotExist, nil, os.ErrNotExist},
			exp:         map[string]string{"apiserver": "kube-apiserver", "thing": "another/thing"},
		},
		{
			// Default thing to component name
			config: map[string]interface{}{
				"components": []string{"apiserver", "thing"},
				"apiserver":  map[string]interface{}{"confs": []string{"apiserver", "kube-apiserver"}},
				"thing":      map[string]interface{}{"confs": []string{"/my/file/thing"}},
			},
			statResults: []error{os.ErrNotExist, nil, os.ErrNotExist},
			exp:         map[string]string{"apiserver": "kube-apiserver", "thing": "thing"},
		},
	}

	v := viper.New()
	statFunc = fakestat

	for id, c := range cases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			for k, val := range c.config {
				v.Set(k, val)
			}
			e = c.statResults
			eIndex = 0

			m := getFiles(v, "config")
			if !reflect.DeepEqual(m, c.exp) {
				t.Fatalf("Got %v\nExpected %v", m, c.exp)
			}
		})
	}
}

func TestGetServiceFiles(t *testing.T) {
	cases := []struct {
		config      map[string]interface{}
		exp         map[string]string
		statResults []error
	}{
		{
			config: map[string]interface{}{
				"components": []string{"kubelet"},
				"kubelet":    map[string]interface{}{"svc": []string{"kubelet", "10-kubeadm.conf"}},
			},
			statResults: []error{os.ErrNotExist, nil},
			exp:         map[string]string{"kubelet": "10-kubeadm.conf"},
		},
		{
			// Component "thing" isn't included in the list of components
			config: map[string]interface{}{
				"components": []string{"kubelet"},
				"kubelet":    map[string]interface{}{"svc": []string{"kubelet", "10-kubeadm.conf"}},
				"thing":      map[string]interface{}{"svc": []string{"/my/file/thing"}},
			},
			statResults: []error{os.ErrNotExist, nil},
			exp:         map[string]string{"kubelet": "10-kubeadm.conf"},
		},
		{
			// More than one component
			config: map[string]interface{}{
				"components": []string{"kubelet", "thing"},
				"kubelet":    map[string]interface{}{"svc": []string{"kubelet", "10-kubeadm.conf"}},
				"thing":      map[string]interface{}{"svc": []string{"/my/file/thing"}},
			},
			statResults: []error{os.ErrNotExist, nil, nil},
			exp:         map[string]string{"kubelet": "10-kubeadm.conf", "thing": "/my/file/thing"},
		},
		{
			// Default thing to specified default service
			config: map[string]interface{}{
				"components": []string{"kubelet", "thing"},
				"kubelet":    map[string]interface{}{"svc": []string{"kubelet", "10-kubeadm.conf"}},
				"thing":      map[string]interface{}{"svc": []string{"/my/file/thing"}, "defaultsvc": "another/thing"},
			},
			statResults: []error{os.ErrNotExist, nil, os.ErrNotExist},
			exp:         map[string]string{"kubelet": "10-kubeadm.conf", "thing": "another/thing"},
		},
		{
			// Default thing to component name
			config: map[string]interface{}{
				"components": []string{"kubelet", "thing"},
				"kubelet":    map[string]interface{}{"svc": []string{"kubelet", "10-kubeadm.conf"}},
				"thing":      map[string]interface{}{"svc": []string{"/my/file/thing"}},
			},
			statResults: []error{os.ErrNotExist, nil, os.ErrNotExist},
			exp:         map[string]string{"kubelet": "10-kubeadm.conf", "thing": "thing"},
		},
	}

	v := viper.New()
	statFunc = fakestat

	for id, c := range cases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			for k, val := range c.config {
				v.Set(k, val)
			}
			e = c.statResults
			eIndex = 0

			m := getFiles(v, "service")
			if !reflect.DeepEqual(m, c.exp) {
				t.Fatalf("Got %v\nExpected %v", m, c.exp)
			}
		})
	}
}

func TestGetDatadirFiles(t *testing.T) {
	var err error
	datadir, err := os.MkdirTemp("", "kube-bench-test-etcd-data-dir")
	if err != nil {
		t.Fatalf("Failed to create temp directory")
	}
	defer os.RemoveAll(datadir)

	cases := []struct {
		config      map[string]interface{}
		exp         map[string]string
		statResults []error
	}{
		{
			config: map[string]interface{}{
				"components": []string{"etcd"},
				"etcd": map[string]interface{}{"datadirs": []string{datadir},
					"defaultdatadir": "/var/lib/etcd/default.etcd"},
			},
			statResults: []error{nil},
			exp:         map[string]string{"etcd": datadir},
		},
		// fallback to defaultdatadir
		{
			config: map[string]interface{}{
				"components": []string{"etcd"},
				"etcd": map[string]interface{}{"datadirs": []string{"/path/to/etcd/data.etcd"},
					"defaultdatadir": "/var/lib/etcd/default.etcd"},
			},
			statResults: []error{os.ErrNotExist},
			exp:         map[string]string{"etcd": "/var/lib/etcd/default.etcd"},
		},
	}

	v := viper.New()
	statFunc = fakestat

	for id, c := range cases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			for k, val := range c.config {
				v.Set(k, val)
			}
			e = c.statResults
			eIndex = 0
			m := getFiles(v, "datadir")
			if !reflect.DeepEqual(m, c.exp) {
				t.Fatalf("Got %v\nExpected %v", m, c.exp)
			}
		})
	}
}

func TestMakeSubsitutions(t *testing.T) {
	cases := []struct {
		input        string
		subst        map[string]string
		exp          string
		expectedSubs []string
	}{
		{input: "Replace $thisbin", subst: map[string]string{"this": "that"}, exp: "Replace that", expectedSubs: []string{"that"}},
		{input: "Replace $thisbin", subst: map[string]string{"this": "that", "here": "there"}, exp: "Replace that", expectedSubs: []string{"that"}},
		{input: "Replace $thisbin and $herebin", subst: map[string]string{"this": "that", "here": "there"}, exp: "Replace that and there", expectedSubs: []string{"that", "there"}},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			s, subs := makeSubstitutions(c.input, "bin", c.subst)
			if s != c.exp {
				t.Fatalf("Got %s expected %s", s, c.exp)
			}
			sort.Strings(subs)
			assert.Equal(t, c.expectedSubs, subs)
		})
	}
}

func TestGetConfigFilePath(t *testing.T) {
	var err error
	cfgDir, err = os.MkdirTemp("", "kube-bench-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory")
	}
	defer os.RemoveAll(cfgDir)
	d := filepath.Join(cfgDir, "cis-1.4")
	err = os.Mkdir(d, 0766)
	if err != nil {
		t.Fatalf("Failed to create temp dir")
	}
	err = os.WriteFile(filepath.Join(d, "master.yaml"), []byte("hello world"), 0666)
	if err != nil {
		t.Logf("Failed to create temp file")
	}

	cases := []struct {
		benchmarkVersion string
		succeed          bool
		exp              string
	}{
		{benchmarkVersion: "cis-1.4", succeed: true, exp: d},
		{benchmarkVersion: "cis-1.5", succeed: false, exp: ""},
		{benchmarkVersion: "1.1", succeed: false, exp: ""},
	}

	for _, c := range cases {
		t.Run(c.benchmarkVersion, func(t *testing.T) {
			path, err := getConfigFilePath(c.benchmarkVersion, "/master.yaml")
			if c.succeed {
				if err != nil {
					t.Fatalf("Error %v", err)
				}
				if path != c.exp {
					t.Fatalf("Got %s expected %s", path, c.exp)
				}
			} else {
				if err == nil {
					t.Fatalf("Expected Error, but none")
				}
			}
		})
	}
}

func TestDecrementVersion(t *testing.T) {
	cases := []struct {
		kubeVersion string
		succeed     bool
		exp         string
	}{
		{kubeVersion: "1.13", succeed: true, exp: "1.12"},
		{kubeVersion: "1.15", succeed: true, exp: "1.14"},
		{kubeVersion: "1.11", succeed: true, exp: "1.10"},
		{kubeVersion: "1.1", succeed: true, exp: ""},
		{kubeVersion: "invalid", succeed: false, exp: ""},
	}
	for _, c := range cases {
		rv := decrementVersion(c.kubeVersion)
		if c.succeed {
			if c.exp != rv {
				t.Fatalf("decrementVersion(%q) - Got %q expected %s", c.kubeVersion, rv, c.exp)
			}
		} else {
			if len(rv) > 0 {
				t.Fatalf("decrementVersion(%q) - Expected empty string but Got %s", c.kubeVersion, rv)
			}
		}
	}
}

func TestGetYamlFilesFromDir(t *testing.T) {
	cfgDir, err := os.MkdirTemp("", "kube-bench-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory")
	}
	defer os.RemoveAll(cfgDir)

	d := filepath.Join(cfgDir, "cis-1.4")
	err = os.Mkdir(d, 0766)
	if err != nil {
		t.Fatalf("Failed to create temp dir")
	}

	err = os.WriteFile(filepath.Join(d, "something.yaml"), []byte("hello world"), 0666)
	if err != nil {
		t.Fatalf("error writing file %v", err)
	}
	err = os.WriteFile(filepath.Join(d, "config.yaml"), []byte("hello world"), 0666)
	if err != nil {
		t.Fatalf("error writing file %v", err)
	}

	files, err := getYamlFilesFromDir(d)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected to find one file, found %d", len(files))
	}

	if files[0] != filepath.Join(d, "something.yaml") {
		t.Fatalf("Expected to find something.yaml, found %s", files[0])
	}
}

func Test_getPlatformNameFromKubectlOutput(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want Platform
	}{
		{
			name: "eks",
			args: args{s: "v1.17.9-eks-4c6976"},
			want: Platform{Name: "eks", Version: "1.17"},
		},
		{
			name: "gke",
			args: args{s: "v1.17.6-gke.1"},
			want: Platform{Name: "gke", Version: "1.17"},
		},
		{
			name: "ack",
			args: args{s: "v1.18.8-aliyun.1"},
			want: Platform{Name: "aliyun", Version: "1.18"},
		},
		{
			name: "unknown",
			args: args{s: "v1.17.6"},
			want: Platform{},
		},
		{
			name: "empty string",
			args: args{s: ""},
			want: Platform{},
		},
		{
			name: "k3s",
			args: args{s: "v1.27.6+k3s1"},
			want: Platform{Name: "k3s", Version: "1.27"},
		},
		{
			name: "rancher1",
			args: args{s: "v1.25.13-rancher1-1"},
			want: Platform{Name: "rancher1", Version: "1.25"},
		},
		{
			name: "rke2",
			args: args{s: "v1.27.6+rke2r1"},
			want: Platform{Name: "rke2r", Version: "1.27"},
		},
		{
			name: "aks",
			args: args{s: "v1.27.6+aks1"},
			want: Platform{Name: "aks", Version: "1.27"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPlatformInfoFromVersion(tt.args.s)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_getPlatformBenchmarkVersion(t *testing.T) {
	type args struct {
		platform Platform
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "eks",
			args: args{
				platform: Platform{Name: "eks"},
			},
			want: "eks-1.2.0",
		},
		{
			name: "gke 1.19",
			args: args{
				platform: Platform{Name: "gke", Version: "1.19"},
			},
			want: "gke-1.0",
		},
		{
			name: "gke 1.20",
			args: args{
				platform: Platform{Name: "gke", Version: "1.20"},
			},
			want: "gke-1.2.0",
		},
		{
			name: "gke 1.22",
			args: args{
				platform: Platform{Name: "gke", Version: "1.22"},
			},
			want: "gke-1.2.0",
		},
		{
			name: "aliyun",
			args: args{
				platform: Platform{Name: "aliyun"},
			},
			want: "ack-1.0",
		},
		{
			name: "unknown",
			args: args{
				platform: Platform{Name: "rh"},
			},
			want: "",
		},
		{
			name: "empty",
			args: args{
				platform: Platform{},
			},
			want: "",
		},
		{
			name: "openshift3",
			args: args{
				platform: Platform{Name: "ocp", Version: "3.10"},
			},
			want: "rh-0.7",
		},
		{
			name: "openshift4",
			args: args{
				platform: Platform{Name: "ocp", Version: "4.1"},
			},
			want: "rh-1.0",
		},
		{
			name: "k3s",
			args: args{
				platform: Platform{Name: "k3s", Version: "1.27"},
			},
			want: "k3s-cis-1.7",
		},
		{
			name: "rancher1",
			args: args{
				platform: Platform{Name: "rancher", Version: "1.27"},
			},
			want: "rke-cis-1.7",
		},
		{
			name: "rke2",
			args: args{
				platform: Platform{Name: "rke2r", Version: "1.27"},
			},
			want: "rke2-cis-1.7",
		},
		{
			name: "aks",
			args: args{
				platform: Platform{Name: "aks", Version: "1.27"},
			},
			want: "aks-1.7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPlatformBenchmarkVersion(tt.args.platform); got != tt.want {
				t.Errorf("getPlatformBenchmarkVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getOcpValidVersion(t *testing.T) {
	cases := []struct {
		openShiftVersion string
		succeed          bool
		exp              string
	}{
		{openShiftVersion: "3.11", succeed: true, exp: "3.10"},
		{openShiftVersion: "3.10", succeed: true, exp: "3.10"},
		{openShiftVersion: "2.9", succeed: false, exp: ""},
		{openShiftVersion: "4.1", succeed: true, exp: "4.1"},
		{openShiftVersion: "4.5", succeed: true, exp: "4.1"},
		{openShiftVersion: "4.6", succeed: true, exp: "4.1"},
		{openShiftVersion: "invalid", succeed: false, exp: ""},
	}
	for _, c := range cases {
		ocpVer, _ := getOcpValidVersion(c.openShiftVersion)
		if c.succeed {
			if c.exp != ocpVer {
				t.Errorf("getOcpValidVersion(%q) - Got %q expected %s", c.openShiftVersion, ocpVer, c.exp)
			}
		} else {
			if len(ocpVer) > 0 {
				t.Errorf("getOcpValidVersion(%q) - Expected empty string but Got %s", c.openShiftVersion, ocpVer)
			}
		}
	}
}
