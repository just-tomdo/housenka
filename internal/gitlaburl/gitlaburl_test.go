package gitlaburl

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Ref
	}{
		{
			name: "work item",
			in:   "https://gitlab.example.com/clients/acme-web/-/work_items/636",
			want: Ref{Host: "gitlab.example.com", ProjectPath: "clients/acme-web", Group: "clients", Repo: "acme-web", Kind: WorkItem, IID: 636},
		},
		{
			name: "issue with note fragment and whitespace",
			in:   "  https://gitlab.example.com/clients/shop-web/-/work_items/393#note_123456 ",
			want: Ref{Host: "gitlab.example.com", ProjectPath: "clients/shop-web", Group: "clients", Repo: "shop-web", Kind: WorkItem, IID: 393},
		},
		{
			name: "classic issue, nested group, upper-case host",
			in:   "https://GitLab.Example.com/interni/backend/shop/-/issues/12",
			want: Ref{Host: "gitlab.example.com", ProjectPath: "interni/backend/shop", Group: "interni/backend", Repo: "shop", Kind: Issue, IID: 12},
		},
		{
			name: "merge request with trailing slash and query",
			in:   "https://gitlab.example.com/clients/beta-app/-/merge_requests/7/?tab=notes",
			want: Ref{Host: "gitlab.example.com", ProjectPath: "clients/beta-app", Group: "clients", Repo: "beta-app", Kind: MergeRequest, IID: 7},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got.Raw = ""
			if got != tt.want {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParseKeepsRaw(t *testing.T) {
	in := "https://gitlab.example.com/clients/shop-web/-/work_items/393#note_123456"
	got, err := Parse(" " + in + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.Raw != in {
		t.Errorf("Raw = %q, want %q", got.Raw, in)
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{
		"",
		"gitlab.example.com/clients/beta-app/-/issues/1",
		"ftp://gitlab.example.com/clients/beta-app/-/issues/1",
		"https:///clients/beta-app/-/issues/1",
		"https://gitlab.example.com/clients/beta-app/issues/1",
		"https://gitlab.example.com/clients/beta-app",
		"https://gitlab.example.com/beta-app/-/issues/1",
		"https://gitlab.example.com/clients//beta-app/-/issues/1",
		"https://gitlab.example.com/clients/beta-app/-/issues/",
		"https://gitlab.example.com/clients/beta-app/-/issues/abc",
		"https://gitlab.example.com/clients/beta-app/-/issues/0",
		"https://gitlab.example.com/clients/beta-app/-/issues/+5",
		"https://gitlab.example.com/clients/beta-app/-/issues/1/designs",
		"https://gitlab.example.com/clients/beta-app/-/pipelines/1",
		"https://gitlab.example.com/clients/beta-app/-/issues",
	} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) expected error", in)
		}
	}
}

func TestShort(t *testing.T) {
	r := Ref{Group: "clients", Repo: "acme-web", Kind: WorkItem, IID: 636}
	if got := r.Short(); got != "clients / acme-web / #636" {
		t.Errorf("Short() = %q", got)
	}
	r.Kind = MergeRequest
	if got := r.Short(); got != "clients / acme-web / !636" {
		t.Errorf("Short() MR = %q", got)
	}
}
