package source

import "github.com/simonwistow/soundscape/internal/spec"

// Spec is one --source (or a mapping's source:), such as wikipedia,
// fastly:service=SID or file:monday.jsonl,loop=true (see package spec for
// the syntax).
type Spec = spec.Spec

var sources = spec.Set{Flag: "--source", Noun: "source", Kinds: []spec.Kind{
	{Name: "simulate", Help: "a built-in, slowly cycling simulation"},
	{Name: "fastly", Options: []string{"service"}, Help: "Fastly's real-time analytics; service defaults to FASTLY_SERVICE_ID, and the token comes from --token or FASTLY_API_TOKEN"},
	{Name: "prometheus", Target: "URL", OptionalTarget: true, Options: []string{"interval"}, Help: "PromQL queries from a mapping, polled every interval (default 1s); URL defaults to PROMETHEUS_URL, else http://localhost:9090"},
	{Name: "wikipedia", Options: []string{"wikis"}, Help: "Wikimedia's recent changes; wikis limits it to some, joined with +, e.g. wikis=enwiki+dewiki"},
	{Name: "file", Target: "PATH", Options: []string{"format", "logformat", "loop"}, Help: "replay recorded telemetry - CSV, JSON Lines, Influx line protocol or Prometheus/OpenMetrics text - or count an access log or packet capture, by extension or format=csv|jsonl|influx|prometheus|apache|pcap; logformat= gives an access log's Apache LogFormat if it isn't Common or Combined; loop=true repeats it forever"},
}}

// Kinds returns the source kinds, for messages.
func Kinds() []string { return sources.Names() }

// Help describes every kind of --source, for the flag's usage.
func Help() string { return sources.Help() }

// ParseSpec parses one --source value, or a mapping's source:.
func ParseSpec(s string) (Spec, error) { return sources.Parse(s) }
