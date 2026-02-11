package framework

import "testing"

func TestTopicMatches(t *testing.T) {
	cases := []struct {
		subscription string
		topic        string
		want         bool
	}{
		{subscription: "state/device-1", topic: "state/device-1", want: true},
		{subscription: "state/device-1", topic: "state/device-2", want: false},
		{subscription: "state/*", topic: "state/device-1", want: true},
		{subscription: "state/*", topic: "state/device-1/entity-1", want: true},
		{subscription: "state/*", topic: "commands/device-1", want: false},
		{subscription: "commands/*", topic: "commands/device-1", want: true},
	}

	for _, tc := range cases {
		if got := topicMatches(tc.subscription, tc.topic); got != tc.want {
			t.Fatalf("topicMatches(%q, %q)=%v want %v", tc.subscription, tc.topic, got, tc.want)
		}
	}
}
