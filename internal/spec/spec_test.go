package spec

import (
	"reflect"
	"strings"
	"testing"
)

var set = Set{Flag: "--thing", Noun: "thing", Kinds: []Kind{
	{Name: "plain", Help: "no target, no options"},
	{Name: "opts", Options: []string{"service", "token"}, Help: "options only"},
	{Name: "path", Target: "PATH", Options: []string{"format"}, Help: "a target"},
	{Name: "url", Target: "URL", OptionalTarget: true, Options: []string{"interval"}, Help: "an optional target"},
}}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Spec
	}{
		{"plain", Spec{Kind: "plain"}},
		{"opts", Spec{Kind: "opts"}},
		{"opts:service=abc", Spec{Kind: "opts", Options: map[string]string{"service": "abc"}}},
		{"opts:service=abc,token=x=y", Spec{Kind: "opts", Options: map[string]string{"service": "abc", "token": "x=y"}}},
		{"path:a.csv", Spec{Kind: "path", Target: "a.csv"}},
		{"path:C:/data/a.csv,format=csv", Spec{Kind: "path", Target: "C:/data/a.csv", Options: map[string]string{"format": "csv"}}},
		// A target that looks like key=value is still the target when the
		// kind must have one.
		{"path:interval=1", Spec{Kind: "path", Target: "interval=1"}},
		{"url", Spec{Kind: "url"}},
		{"url:http://host:9090", Spec{Kind: "url", Target: "http://host:9090"}},
		{"url:http://host:9090,interval=2s", Spec{Kind: "url", Target: "http://host:9090", Options: map[string]string{"interval": "2s"}}},
		{"url:interval=2s", Spec{Kind: "url", Options: map[string]string{"interval": "2s"}}},
		{"url:http://h/?q=1", Spec{Kind: "url", Target: "http://h/?q=1"}},
	} {
		got, err := set.Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Parse(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"nope", `--thing nope: unknown thing "nope" (want one of plain, opts, path, url)`},
		{"", `unknown thing ""`},
		{"plain:x", "plain takes no target or options"},
		{"plain:x=1", "plain takes no options"},
		{"opts:abc", `option "abc" isn't key=value`},
		{"opts:region=eu", `unknown option "region" for opts (want service, token)`},
		{"opts:service=a,service=b", `option "service" given twice`},
		{"path", "path needs a target: path:PATH"},
		{"path:", "path needs a target"},
	} {
		_, err := set.Parse(tc.in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Parse(%q) error = %v, want it to mention %q", tc.in, err, tc.want)
		}
	}
}

func TestStringRoundTrips(t *testing.T) {
	for _, in := range []string{"plain", "opts:service=a,token=b", "path:a.csv,format=csv", "url:http://h:1,interval=2s", "url:interval=2s"} {
		s, err := set.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.String(); got != in {
			t.Errorf("Parse(%q).String() = %q", in, got)
		}
	}
}

func TestHelp(t *testing.T) {
	help := set.Help()
	for _, want := range []string{
		"\n  plain\n",
		"\n  opts[:service=...][,token=...]\n",
		"\n  path:PATH[,format=...]\n",
		"\n  url[:URL][,interval=...]\n",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("Help() lacks %q:\n%s", want, help)
		}
	}
}
