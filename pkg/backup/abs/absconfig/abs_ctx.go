package absconfig

import "errors"

// ABSContext represents the controller context around a ABS backend configuration
type ABSContext struct {
	ABSSecret    string
	ABSContainer string
}

// Validate validates a given ABSContext
func (a *ABSContext) Validate() error {
	allEmpty := len(a.ABSSecret) == 0 && len(a.ABSContainer) == 0
	allSet := len(a.ABSSecret) != 0 && len(a.ABSContainer) != 0

	if !(allEmpty || allSet) {
		return errors.New("ABS related values should be all set or all empty")
	}

	return nil
}
