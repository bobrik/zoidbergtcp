package application

import (
	"fmt"
)

// Finder finds apps
type Finder interface {
	Apps() (Apps, error)
}

// FinderMaker represents finder maker tuple:
// * a function to register flags
// * a function to make finder from parsed flags and balancer name
type FinderMaker struct {
	Flags func()
	Maker func(balancer string) (Finder, error)
}

// finderMakers contains a mapping of Finder names to their makers
var finderMakers = map[string]FinderMaker{}

// RegisterFinderMaker registers a new finder maker with a name
func RegisterFinderMaker(name string, maker FinderMaker) {
	finderMakers[name] = maker
}

// FinderByName returns existing finder by name a balancer name
func FinderByName(finder string, balancer string) (Finder, error) {
	if maker, ok := finderMakers[finder]; ok {
		return maker.Maker(balancer)
	}

	return nil, fmt.Errorf("unknown application finder: %q", finder)
}

// RegisterFlags registers flags of all finder makers
func RegisterFlags() {
	for _, m := range finderMakers {
		m.Flags()
	}
}
