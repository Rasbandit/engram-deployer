package deploy

import (
	"strings"
	"testing"
)

const sampleTemplate = `<?xml version="1.0"?>
<Container version="2">
  <Name>engram-saas</Name>
  <Repository>ghcr.io/engram-app/engram:0.5.60</Repository>
  <Registry>https://ghcr.io/</Registry>
  <Network>bridge</Network>
</Container>
`

func TestReplaceRepoTag_ReplacesAndPreservesRest(t *testing.T) {
	got, err := ReplaceRepoTag([]byte(sampleTemplate), "ghcr.io/engram-app/engram", "0.5.61")
	if err != nil {
		t.Fatalf("ReplaceRepoTag: %v", err)
	}

	if !strings.Contains(string(got), "<Repository>ghcr.io/engram-app/engram:0.5.61</Repository>") {
		t.Errorf("repository tag not updated; got:\n%s", got)
	}
	if strings.Contains(string(got), "0.5.60") {
		t.Errorf("old version still present in output; got:\n%s", got)
	}
	// Other tags untouched.
	for _, untouched := range []string{
		"<Name>engram-saas</Name>",
		"<Registry>https://ghcr.io/</Registry>",
		"<Network>bridge</Network>",
	} {
		if !strings.Contains(string(got), untouched) {
			t.Errorf("expected unchanged line missing: %s", untouched)
		}
	}
}

func TestReplaceRepoTag_Idempotent(t *testing.T) {
	once, err := ReplaceRepoTag([]byte(sampleTemplate), "ghcr.io/engram-app/engram", "0.5.61")
	if err != nil {
		t.Fatal(err)
	}
	twice, err := ReplaceRepoTag(once, "ghcr.io/engram-app/engram", "0.5.61")
	if err != nil {
		t.Fatal(err)
	}
	if string(once) != string(twice) {
		t.Errorf("not idempotent:\nonce:\n%s\ntwice:\n%s", once, twice)
	}
}

func TestReplaceRepoTag_FailsIfNoRepositoryTag(t *testing.T) {
	bogus := []byte(`<?xml version="1.0"?><Container><Name>x</Name></Container>`)
	if _, err := ReplaceRepoTag(bogus, "ghcr.io/engram-app/engram", "0.5.61"); err == nil {
		t.Fatal("expected error when <Repository> tag missing")
	}
}

func TestReplaceRepoTag_FailsIfRepositoryDoesntMatchImage(t *testing.T) {
	// Wrong image (e.g. someone passed the saas image to the selfhost template
	// by mistake). Refuse loudly rather than silently fail to match.
	mismatched := strings.ReplaceAll(
		sampleTemplate,
		"ghcr.io/engram-app/engram:0.5.60",
		"ghcr.io/some-other/image:1.0.0",
	)
	if _, err := ReplaceRepoTag([]byte(mismatched), "ghcr.io/engram-app/engram", "0.5.61"); err == nil {
		t.Fatal("expected error when <Repository> doesn't reference the target image")
	}
}
