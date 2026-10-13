package main

import "testing"

func TestCombineURL(t *testing.T) {
	tests := []struct {
		name    string
		rootURL string
		subURL  string
		want    string
		wantErr bool
	}{
		{
			name:    "host only",
			rootURL: "http://192.168.1.1:1234",
			subURL:  "control/ctl",
			want:    "http://192.168.1.1:1234/control/ctl",
		},
		{
			name:    "host with slash",
			rootURL: "http://192.168.1.1/",
			subURL:  "control/ctl",
			want:    "http://192.168.1.1/control/ctl",
		},
		{
			name:    "port",
			rootURL: "http://192.168.1.1:1234/foo/",
			subURL:  "ctl",
			want:    "http://192.168.1.1:1234/foo/ctl",
		},
		{
			name:    "absolute path",
			rootURL: "http://host/foo/bar",
			subURL:  "/ctl",
			want:    "http://host/ctl",
		},
		{
			name:    "relative path",
			rootURL: "http://host/foo/",
			subURL:  "ctl",
			want:    "http://host/foo/ctl",
		},
		{
			name:    "whitespace around relative path",
			rootURL: "http://host/foo/",
			subURL:  " \nctl\t ",
			want:    "http://host/foo/ctl",
		},
		{
			name:    "parent path",
			rootURL: "http://host/foo/bar",
			subURL:  "../ctl",
			want:    "http://host/ctl",
		},
		{
			name:    "empty reference",
			rootURL: "http://host/foo/bar",
			want:    "http://host/foo/bar",
		},
		{
			name:    "absolute URL",
			rootURL: "http://host/foo/bar",
			subURL:  "https://other.example/ctl",
			want:    "https://other.example/ctl",
		},
		{
			name:    "invalid URL escape",
			rootURL: "http://host/foo/",
			subURL:  "%zz",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := combineURL(test.rootURL, test.subURL)
			if (err != nil) != test.wantErr {
				t.Fatalf("combineURL() error = %v, wantErr %t", err, test.wantErr)
			}
			if err != nil {
				return
			}
			if got != test.want {
				t.Errorf("combineURL() = %q, want %q", got, test.want)
			}
		})
	}
}
