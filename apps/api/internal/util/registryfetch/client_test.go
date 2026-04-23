package registryfetch

import "testing"

func TestNextCursor(t *testing.T) {
	cases := []struct {
		name, link, want string
	}{
		{"empty", "", ""},
		{"no rel-next", `<http://host/x>; rel="first"`, ""},
		{"full url", `<http://host/v2/_catalog?n=100&last=foo>; rel="next"`, "foo"},
		{"path only", `</v2/_catalog?n=100&last=foo%2Fbar>; rel="next"`, "foo/bar"},
		{"no last", `</v2/_catalog?n=100>; rel="next"`, ""},
		{"malformed", `<::not a url::>; rel="next"`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nextCursor(c.link); got != c.want {
				t.Errorf("nextCursor(%q) = %q, want %q", c.link, got, c.want)
			}
		})
	}
}

func TestManifestIsList(t *testing.T) {
	cases := []struct {
		name string
		m    Manifest
		want bool
	}{
		{"empty", Manifest{}, false},
		{"single arch by layers", Manifest{Layers: []Descriptor{{Size: 1}}}, false},
		{"list by manifests slice", Manifest{Manifests: []ManifestListEntry{{Digest: "sha256:x"}}}, true},
		{"list by docker mediatype", Manifest{MediaType: MediaTypeDockerManifestList}, true},
		{"list by oci mediatype", Manifest{MediaType: MediaTypeOCIImageIndex}, true},
		{"v2 single not list", Manifest{MediaType: MediaTypeDockerManifestV2}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.m.IsList(); got != c.want {
				t.Errorf("IsList = %v, want %v", got, c.want)
			}
		})
	}
}
