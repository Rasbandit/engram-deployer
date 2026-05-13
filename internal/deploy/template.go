package deploy

import (
	"bytes"
	"fmt"
	"regexp"
)

// ReplaceRepoTag rewrites the <Repository>image:OLD</Repository> tag inside
// an Unraid container template XML to image:version, preserving whitespace
// and all other content byte-for-byte.
//
// Returns an error if:
//   - no <Repository> tag found at all
//   - the existing <Repository> tag references a different image than `image`
//
// Both cases indicate the caller pointed at the wrong file or wrong target;
// either silently editing or no-op'ing would mask a deploy misconfiguration.
func ReplaceRepoTag(xml []byte, image, version string) ([]byte, error) {
	// Match: <Repository>{image}:{anything-not-<}</Repository>
	// Anchored on the image so a mismatched image doesn't match.
	pattern := regexp.MustCompile(
		`<Repository>` + regexp.QuoteMeta(image) + `:[^<]*</Repository>`,
	)
	if !pattern.Match(xml) {
		// Distinguish "no Repository tag" from "wrong image".
		if !bytes.Contains(xml, []byte("<Repository>")) {
			return nil, fmt.Errorf("no <Repository> tag found in template")
		}
		return nil, fmt.Errorf("<Repository> tag does not reference image %q", image)
	}
	replacement := []byte("<Repository>" + image + ":" + version + "</Repository>")
	return pattern.ReplaceAll(xml, replacement), nil
}
