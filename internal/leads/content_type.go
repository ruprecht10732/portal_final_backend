package leads

import "strings"

func isImageContentType(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}