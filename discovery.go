package startup

import (
	"errors"
	"sync"
)

// DiscoveryService provides a mechanism for registry and discovery of Identities
type DiscoveryService interface {
	// Register allows Identities to be declared.  An error is raised if the Identity is already declared
	Register(id Identity) error
	// Find allows the Location of a given ID to be retrieved, for subsequent Connection attempts
	Find(id string) (Location, error)
}

// NewDiscoveryService returns an empty instance of DiscoveryService
func NewDiscoveryService() DiscoveryService {
	return &ds{
		m: map[string]Identity{},
	}
}

type ds struct {
	m   map[string]Identity
	lck sync.Mutex
}

// ErrNilID returned if the Identity has no id specified
var ErrNilID = errors.New("id must not be nil")

// ErrInvalidID returned if the Identity is invalid
var ErrInvalidID = errors.New("invalid id")

// ErrIDNotFound returned if the Identity is not in the Discovery Service
var ErrIDNotFound = errors.New("id is not registered")

// ErrIDAlreadyRegistered returned if the Identity is already registered in the Discovery Service
var ErrIDAlreadyRegistered = errors.New("id is already registered")

func (d *ds) Register(id Identity) error {
	if id == nil {
		return ErrNilID
	}

	d.lck.Lock()
	defer d.lck.Unlock()

	if _, ok := d.m[id.ID()]; ok {
		return ErrIDAlreadyRegistered
	}

	d.m[id.ID()] = id
	return nil
}

func (d *ds) Find(id string) (Location, error) {
	if len(id) == 0 {
		return nil, ErrInvalidID
	}

	d.lck.Lock()
	defer d.lck.Unlock()

	if i, ok := d.m[id]; !ok {
		return nil, ErrIDNotFound
	} else {
		return i.Loc(), nil
	}
}
