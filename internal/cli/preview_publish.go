package cli

// renamePreviewExclusiveAtBoundary runs the platform's atomic no-replace
// operation. beforePublish is an unexported test seam that can install a
// competing destination immediately before that operation.
func renamePreviewExclusiveAtBoundary(stage, out string, beforePublish func()) error {
	if beforePublish != nil {
		beforePublish()
	}
	return renamePreviewExclusiveNative(stage, out)
}
