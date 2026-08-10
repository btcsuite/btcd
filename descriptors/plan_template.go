package descriptors

import "bytes"

// plannedLookup resolves one signature or preimage of a fixed spending path.
type plannedLookup func(*Satisfier) ([]byte, bool)

// planScript selects a satisfaction once and records how each of its variable
// elements is obtained. Completion fills only those elements: it must not run
// path selection again with different signature lengths or availability.
// planSign must return a fresh, nonempty allocation for each available
// signature. The returned template is for sizing, not script execution.
func (d *Descriptor) planScript(n *node, mp, idx uint32, assets Assets,
	planSign func(string) ([]byte, bool),
	realSign func(*Satisfier) func(string) ([]byte, bool)) ([][]byte,
	func(*Satisfier) ([][]byte, error), error) {

	// The miniscript satisfier reorders and concatenates stacks but
	// preserves each element's backing slice. Use that identity to
	// distinguish lookup placeholders from literal keys, selectors and
	// dissatisfactions, even when their contents are equal. Every
	// placeholder is nonempty and has its own allocation; no magic byte
	// value is reserved in the script.
	lookups := make(map[*byte]plannedLookup)
	sign := func(key string) ([]byte, bool) {
		value, ok := planSign(key)
		if !ok {
			return nil, false
		}
		lookups[&value[0]] = func(s *Satisfier) ([]byte, bool) {
			return realSign(s)(key)
		}
		return value, true
	}
	preimage := func(function string, hash []byte) ([]byte, bool) {
		value, ok := planPreimage(assets)(function, hash)
		if !ok {
			return nil, false
		}
		hash = bytes.Clone(hash)
		lookups[&value[0]] = func(s *Satisfier) ([]byte, bool) {
			result, found := realPreimage(s)(function, hash)
			return result, found && len(result) == hashPreimageLen
		}
		return value, true
	}
	template, err := d.satisfyScript(n, mp, idx, assets, sign, preimage)
	if err != nil {
		return nil, nil, err
	}

	// Retain only the lookups actually chosen. Literal elements are copied
	// both now and on completion so a caller cannot mutate the saved plan.
	selected := make([]plannedLookup, len(template))
	for i, item := range template {
		if len(item) != 0 {
			selected[i] = lookups[&item[0]]
		}
		if selected[i] == nil {
			literal := bytes.Clone(item)
			selected[i] = func(*Satisfier) ([]byte, bool) {
				return literal, true
			}
		}
	}

	// Materialize the recorded path without consulting availability again.
	// Copy callback results so callers cannot mutate literals retained by
	// the plan through a completed witness.
	satisfy := func(s *Satisfier) ([][]byte, error) {
		stack := make([][]byte, len(selected))
		for i, lookup := range selected {
			item, ok := lookup(s)
			if !ok {
				return nil, errCouldNotSatisfy
			}
			stack[i] = bytes.Clone(item)
		}
		return stack, nil
	}
	return template, satisfy, nil
}
