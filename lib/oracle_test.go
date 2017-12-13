package mporacle

import (
	"testing"
)

func TestWaitEventNameEmpty(t *testing.T) {
	var opt waitEventNames
	err := opt.Set("")
	if err == nil {
		t.Fatal("should be an error")
	}
	err = opt.Set("foo")
	if err != nil {
		t.Fatalf("should not be an error: %v", err)
	}
}

func TestWaitEventNameString(t *testing.T) {
	var opt waitEventNames
	opt.Set(`foo`)
	opt.Set(`bar`)
	opt.Set(`"baz"`)
	got := opt.String()
	want := `"foo","bar","\"baz\""`
	if got != want {
		t.Fatalf("want %q but got %v", want, got)
	}
}

func TestWaitEventNameMatch(t *testing.T) {
	var opt waitEventNames
	opt.Set(`foo`)
	opt.Set(`bar`)
	opt.Set(`"baz"`)
	opt.Set(`/^(mackerel|agent)$/`)

	tests := []struct {
		name string
		want bool
	}{
		{"foo", true},
		{"bar", true},
		{"baz", false},
		{`"baz"`, true},
		{`macker`, false},
		{`mackerel`, true},
		{`agent`, true},
	}
	for _, tt := range tests {
		want := opt.Match(tt.name)
		if want != tt.want {
			t.Fatalf("Match(%q) return %v but want %v", tt.name, want, tt.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"foo", "foo"},
		{"f:o", "f_o"},
		{"f/o", "fo"},
		{"f:o", "f_o"},
	}
	for _, tt := range tests {
		want := normalize(tt.name)
		if want != tt.want {
			t.Fatalf("normalize(%q) return %q but want %q", tt.name, want, tt.want)
		}
	}
}
