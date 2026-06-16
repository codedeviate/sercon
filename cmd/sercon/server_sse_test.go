package main

import "testing"

func TestFormatSSEEvent(t *testing.T) {
	cases := []struct {
		name string
		ev   sseEvent
		want string
	}{
		{"data only", sseEvent{data: "hello"}, "data: hello\n\n"},
		{"named with id", sseEvent{event: "tick", data: "1", id: "42"}, "id: 42\nevent: tick\ndata: 1\n\n"},
		{"multiline data", sseEvent{data: "a\nb"}, "data: a\ndata: b\n\n"},
		{"crlf normalized", sseEvent{data: "a\r\nb"}, "data: a\ndata: b\n\n"},
		{"retry hint", sseEvent{data: "x", retry: 3000}, "retry: 3000\ndata: x\n\n"},
		{"empty data", sseEvent{data: ""}, "data: \n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(formatSSEEvent(tc.ev))
			if got != tc.want {
				t.Fatalf("formatSSEEvent(%+v) = %q, want %q", tc.ev, got, tc.want)
			}
		})
	}
}
