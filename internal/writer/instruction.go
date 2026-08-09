package writer

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

const managedRegionMarkerPrefix = "<!-- sth:managed:"

func renderInstructionFragment(request WriteRequest, targetName string) ([]byte, error) {
	if !utf8.Valid(request.Content) {
		return nil, fmt.Errorf("instruction %s invalide : contenu non UTF-8", targetName)
	}
	if bytes.Contains(request.Content, []byte(managedRegionMarkerPrefix)) {
		return nil, fmt.Errorf("instruction %s invalide : marqueur de région STH réservé", targetName)
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
