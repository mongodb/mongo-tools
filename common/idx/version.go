package idx

var fieldTypeRequiredOpts = []struct {
	fieldType string
	option    string
}{
	{"2dsphere", "2dsphereIndexVersion"},
}

// EnsureIndexVersions ensures that each versioned index has
// a version number. This prevents current MongoDB servers from
// creating current-version indexes when the backed-up index was
// actually old enough to predate index versioning (and is thus
// always version 1).
//
// The returned map maps index properties to their new values
// (as of this writing, always 1).
func (idx IndexDocument) EnsureIndexVersions() map[string]any {
	inferred := map[string]any{}

	for _, keySpec := range idx.Key {
		for _, ftro := range fieldTypeRequiredOpts {
			if keySpec.Value == ftro.fieldType {
				if _, hasOpt := idx.Options[ftro.option]; !hasOpt {
					inferred[ftro.option] = 1
				}
			}
		}
	}

	for optName, optVal := range inferred {
		idx.Options[optName] = optVal
	}

	return inferred
}
