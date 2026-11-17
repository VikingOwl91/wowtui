package service

import (
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/addon"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func inferFolder(name string, interfaces ...string) *addon.Folder {
	f := &addon.Folder{Name: name}
	for _, value := range interfaces {
		f.Tocs = append(f.Tocs, &addon.Toc{Fields: map[string]string{"interface": value}})
	}
	return f
}

func TestInferFlavor(t *testing.T) {
	tests := []struct {
		name    string
		folders []*addon.Folder
		want    wow.GameFlavor
	}{
		{
			name: "retail install with multi-flavor addons",
			folders: []*addon.Folder{
				inferFolder("Details", "110200", "40402"),
				inferFolder("Multi", "110200, 50500, 11507"),
				inferFolder("Plater", "110200"),
				inferFolder("WeakAuras", "110200, 110205"),
			},
			want: wow.Retail,
		},
		{
			name: "classic era majority",
			folders: []*addon.Folder{
				inferFolder("A", "11507"),
				inferFolder("B", "11508"),
				inferFolder("C", "110200"),
				inferFolder("D", "110200, 11507"),
			},
			want: wow.ClassicEra,
		},
		{
			name: "unknown interface numbers are not evidence",
			folders: []*addon.Folder{
				inferFolder("A", "50504, 70000"),
			},
			want: wow.Classic,
		},
		{
			name: "only ambiguous folders",
			folders: []*addon.Folder{
				inferFolder("A", "110200, 11507"),
				inferFolder("B", "110200", "50504"),
			},
			want: "",
		},
		{
			name: "tied evidence",
			folders: []*addon.Folder{
				inferFolder("A", "110200"),
				inferFolder("B", "11507"),
			},
			want: "",
		},
		{
			name: "no interfaces",
			folders: []*addon.Folder{
				inferFolder("A", ""),
				inferFolder("B", "notanumber"),
			},
			want: "",
		},
		{
			name: "no folders",
			want: "",
		},
		{
			name: "tbc interfaces infer anniversary",
			folders: []*addon.Folder{
				inferFolder("A", "20506"),
				inferFolder("B", "20505, 20506"),
			},
			want: wow.Anniversary,
		},
		{
			name: "forever interfaces infer forever",
			folders: []*addon.Folder{
				inferFolder("A", "16001"),
				inferFolder("B", "110200, 11509"),
			},
			want: wow.Forever,
		},
		{
			name: "titan interfaces infer classic-titan",
			folders: []*addon.Folder{
				inferFolder("A", "38002"),
			},
			want: wow.ClassicTitan,
		},
		{
			name: "game types without a live client are not evidence",
			folders: []*addon.Folder{
				inferFolder("A", "30405"),
				inferFolder("B", "40402"),
			},
			want: "",
		},
		{
			name: "folder spanning game types is ambiguous even without a live client",
			folders: []*addon.Folder{
				inferFolder("A", "110200, 30405"),
				inferFolder("B", "110200", "40402"),
				inferFolder("C", "11509"),
			},
			want: wow.ClassicEra,
		},
		{
			name: "vanilla and tbc are different game types",
			folders: []*addon.Folder{
				inferFolder("A", "11509, 20506"),
				inferFolder("B", "20506"),
			},
			want: wow.Anniversary,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferFlavor(tt.folders); got != tt.want {
				t.Errorf("inferFlavor = %q, want %q", got, tt.want)
			}
		})
	}
}
