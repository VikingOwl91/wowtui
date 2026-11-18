package main

import (
	"context"
	"strings"
	"testing"
)

func TestDiscoverHintWinsOverCodeSearch(t *testing.T) {
	w := newWorld()
	w.repo("own/Foo", 5, "")
	w.repo("other/Foo", 6, "")
	w.file("own/Foo", "Foo.toc", tocText("Foo", "110200", "## X-Curse-Project-ID: 5"))
	w.hit("curse", "5", "other/Foo", "Foo.toc")
	reg := testRegistry([]regFolder{
		{name: "Foo", hints: map[string]string{"x-curse-project-id": "5", "x-website": "https://github.com/own/Foo"}},
		{name: "Foo_Options", hints: map[string]string{"x-curse-project-id": "5"}},
	})
	cands, review, _, _ := runDiscover(t, w, reg, t.TempDir(), newFakeClock())
	if len(cands.Packages) != 1 || cands.Packages[0].Repo != "own/Foo" {
		t.Fatalf("packages = %v", packageRepos(cands))
	}
	if item := reviewFor(t, review, reasonHintDisagrees, "Foo"); !strings.Contains(item.Detail, "other/Foo") {
		t.Errorf("item = %+v", item)
	}
}

func TestDiscoverReportsConflictingIDs(t *testing.T) {
	w := newWorld()
	w.repo("a/Foo", 5, "")
	w.repo("b/Foo", 5, "")
	w.hit("curse", "1", "a/Foo", "Foo.toc")
	w.hit("wago", "x", "b/Foo", "Foo.toc")
	reg := testRegistry([]regFolder{{name: "Foo", hints: map[string]string{"x-curse-project-id": "1", "x-wago-id": "x"}}})
	cands, review, _, _ := runDiscover(t, w, reg, t.TempDir(), newFakeClock())
	if len(cands.Packages) != 0 {
		t.Fatalf("packages = %v", packageRepos(cands))
	}
	item := reviewFor(t, review, reasonConflict, "Foo")
	if !strings.Contains(item.Detail, "a/Foo") || !strings.Contains(item.Detail, "b/Foo") {
		t.Errorf("item = %+v", item)
	}
}

func TestDiscoverKeepsGoingAfterSearchErrors(t *testing.T) {
	w := newWorld()
	w.repo("a/Good", 5, "")
	w.hit("curse", "2", "a/Good", "Good.toc")
	w.file("a/Good", "Good.toc", tocText("Good", "110200"))
	reg := testRegistry([]regFolder{
		{name: "Bad", hints: map[string]string{"x-curse-project-id": "1"}},
		{name: "Good", hints: map[string]string{"x-curse-project-id": "2"}},
	})
	gh := &fakeGH{handler: func(req Request) Response {
		if req.Path == "search/code" && strings.Contains(req.Params["q"], ": 1\"") {
			return Response{Status: 422, Body: []byte(`{"message":"Validation Failed"}`)}
		}
		return w.handle(req)
	}}
	c := NewClient(gh, ClientConfig{CacheDir: t.TempDir(), Clock: newFakeClock()})
	cands, review, err := Discover(context.Background(), c, reg, DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands.Packages) != 1 || cands.Packages[0].Repo != "a/Good" {
		t.Fatalf("packages = %v", packageRepos(cands))
	}
	if item := reviewFor(t, review, reasonAPIError, "Bad"); !strings.Contains(item.Detail, "422") {
		t.Errorf("item = %+v", item)
	}
}
