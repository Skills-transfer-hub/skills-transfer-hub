package writer

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

const managedRegionMarkerPrefix = "<!-- sth:managed:"

func renderInstructionFragment(request WriteRequest, targetName string) ([]byte, error) {
	if !utf8.Valid(request.Content) {
		return nil, fmt.Errorf("invalid %s instruction: content is not UTF-8", targetName)
	}
	if bytes.Contains(request.Content, []byte(managedRegionMarkerPrefix)) {
		return nil, fmt.Errorf("invalid %s instruction: reserved STH region marker", targetName)
	}

	content := request.Content
	if bodyOffset := FrontmatterBodyOffset(content); bodyOffset >= 0 {
		content = content[bodyOffset:]
	}
	if request.AddManagedHeader {
		content = applyMinimalManagedMarker(request.InstalledPath, content)
	}
	return content, nil
}
