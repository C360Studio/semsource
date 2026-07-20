package codecontext

import (
	"testing"

	"github.com/c360studio/semstreams/pkg/fusion"
)

func TestDropNavigationalNodes(t *testing.T) {
	tests := []struct {
		name string
		in   []fusion.Node
		want []string
		why  string
	}{
		{
			name: "body-less parent is dropped ahead of the passage that answers",
			in: []fusion.Node{
				{Name: "Guide", Kind: "document", Body: ""},
				{Name: "Guide § Ports", Kind: "passage", Body: "semembed listens on 8081"},
			},
			want: []string{"Guide § Ports"},
			why:  "the defect: an empty citation ranked first",
		},
		{
			name: "passages are never dropped",
			in: []fusion.Node{
				{Name: "a", Kind: "passage", Body: "x"},
				{Name: "b", Kind: "passage", Body: "y"},
			},
			want: []string{"a", "b"},
		},
		{
			name: "a body-bearing document survives",
			in: []fusion.Node{
				{Name: "legacy", Kind: "document", Body: "still has a body"},
			},
			want: []string{"legacy"},
			why:  "filter on declared kind AND emptiness, so a body-bearing doc is not swallowed",
		},
		{
			name: "a body-less PASSAGE survives — that is a fault worth seeing",
			in: []fusion.Node{
				{Name: "broken", Kind: "passage", Body: ""},
				{Name: "ok", Kind: "passage", Body: "z"},
			},
			want: []string{"broken", "ok"},
			why:  "swallowing it would hide an unhydratable passage",
		},
		{
			name: "never filters to empty",
			in: []fusion.Node{
				{Name: "only", Kind: "document", Body: ""},
			},
			want: []string{"only"},
			why:  "an honest thin answer beats a silent nothing",
		},
		{name: "empty input", in: nil, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dropNavigationalNodes(tt.in)
			var names []string
			for _, n := range got {
				names = append(names, n.Name)
			}
			if len(names) != len(tt.want) {
				t.Fatalf("got %v, want %v (%s)", names, tt.want, tt.why)
			}
			for i := range names {
				if names[i] != tt.want[i] {
					t.Errorf("node %d = %q, want %q (%s)", i, names[i], tt.want[i], tt.why)
				}
			}
		})
	}
}
