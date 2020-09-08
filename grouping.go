package rbac

import (
	"fmt"
	"strings"
)

type Subject interface {
	subject() string
}

type User string

func (u User) subject() string {
	return "user:" + string(u)
}

type Role string

func (r Role) subject() string {
	return "role:" + string(r)
}

func ParseSubject(sub string) (Subject, error) {
	if strings.HasPrefix(sub, "user:") {
		u := strings.TrimPrefix(sub, "user:")
		return User(u), nil
	}
	if strings.HasPrefix(sub, "role:") {
		r := strings.TrimPrefix(sub, "role:")
		return Role(r), nil
	}

	return nil, ErrInvlaidSubject
}

type Grouping interface {
	Join(Subject, Role) error
	Leave(Subject, Role) error

	HasRole(User, Role) (bool, error)

	AllRoles() (map[Role]struct{}, error)
	AllUsers() (map[User]struct{}, error)

	RolesOf(User) (map[Role]struct{}, error)
	UsersOf(Role) (map[User]struct{}, error)

	DirectRolesOf(Subject) (map[Role]struct{}, error)
	DirectSubjectsOf(Role) (map[Subject]struct{}, error)

	RemoveRole(Role) error
	RemoveUser(User) error
}

var _ Grouping = (*simpleGrouping)(nil)
var _ Grouping = (*fatGrouping)(nil)

// simpleGrouping stores lest information and does everything in memory
type simpleGrouping struct {
	parents  map[Subject]map[Role]struct{}
	children map[Role]map[Subject]struct{}
	maxDepth int
}

func newSimpleGrouping() *simpleGrouping {
	return &simpleGrouping{
		parents:  make(map[Subject]map[Role]struct{}),
		children: make(map[Role]map[Subject]struct{}),
		maxDepth: 10,
	}
}

// Join implements Grouping interface
func (e *simpleGrouping) Join(sub Subject, role Role) error {
	if e.parents[sub] == nil {
		e.parents[sub] = make(map[Role]struct{}, 1)
	}
	e.parents[sub][role] = struct{}{}

	if e.children[role] == nil {
		e.children[role] = make(map[Subject]struct{})
	}
	e.children[role][sub] = struct{}{}

	return nil
}

// Leave implements Grouping interfaceJ
func (e *simpleGrouping) Leave(sub Subject, role Role) error {
	if e.parents[sub] == nil {
		return fmt.Errorf("%w: grouping rule: %s -> %s", ErrNotFound, sub.subject(), role.subject())
	}
	delete(e.parents[sub], role)

	if e.children[role] == nil {
		return fmt.Errorf("%w: grouping rule: %s -> %s", ErrNotFound, role.subject(), sub.subject())
	}
	delete(e.children[role], sub)

	return nil
}

// HasRole implements Grouping interface
func (e *simpleGrouping) HasRole(user User, role Role) (bool, error) {
	roles, err := e.RolesOf(user)
	if err != nil {
		return false, err
	}
	for r := range roles {
		if r == role {
			return true, nil
		}
	}
	return false, nil
}

// AllRoles implements Grouping interface
func (e *simpleGrouping) AllRoles() (map[Role]struct{}, error) {
	roles := make(map[Role]struct{}, len(e.children))
	for role := range e.children {
		roles[role] = struct{}{}
	}
	return roles, nil
}

// AllUsers implements Grouping interface
func (e *simpleGrouping) AllUsers() (map[User]struct{}, error) {
	users := make(map[User]struct{}, len(e.parents))
	for sub := range e.parents {
		if user, ok := sub.(User); ok {
			users[user] = struct{}{}
		}
	}
	return users, nil
}

// RolesOf implements Grouping interface
func (e *simpleGrouping) RolesOf(user User) (map[Role]struct{}, error) {
	ancients := make(map[Role]struct{})

	var query func(sub Subject, depth int)
	query = func(sub Subject, depth int) {
		if depth > e.maxDepth {
			return
		}
		for r := range e.parents[sub] {
			ancients[r] = struct{}{}
			query(r, depth+1)
		}
	}
	query(user, 0)

	return ancients, nil
}

// UsersOf implements Grouping interface
func (e *simpleGrouping) UsersOf(role Role) (map[User]struct{}, error) {
	children := make(map[User]struct{})

	var query func(role Role, depth int)
	query = func(role Role, depth int) {
		if depth > e.maxDepth {
			return
		}
		for ch := range e.children[role] {
			if user, ok := ch.(User); ok {
				children[user] = struct{}{}
			} else {
				query(ch.(Role), depth+1)
			}
		}
	}
	query(role, 0)

	return children, nil
}

func (e *simpleGrouping) DirectRolesOf(sub Subject) (map[Role]struct{}, error) {
	return e.parents[sub], nil
}

func (e *simpleGrouping) DirectSubjectsOf(role Role) (map[Subject]struct{}, error) {
	return e.children[role], nil
}

// RemoveRole implements Grouping interface
func (e *simpleGrouping) RemoveRole(role Role) error {
	children := e.children[role]
	delete(e.children, role)

	for ch := range children {
		delete(e.parents[ch], role)
	}

	return nil
}

// RemoveUser implements Grouping interface
func (e *simpleGrouping) RemoveUser(user User) error {
	parents := e.parents[user]
	delete(e.parents, user)

	for p := range parents {
		delete(e.children[p], user)
	}
	return nil
}

// fatGrouping stores more information to speed up querying
// fatGrouping is faster on quering, and slower on removing comprared to the innter Grouping
type fatGrouping struct {
	Grouping

	// subject => all roles it belongs to
	roles map[Subject]map[Role]struct{}
	// role => all users belongs to it
	users map[Role]map[User]struct{}
}

func newFatGrouping(g Grouping) *fatGrouping {
	return &fatGrouping{
		Grouping: g,
		roles:    make(map[Subject]map[Role]struct{}),
		users:    make(map[Role]map[User]struct{}),
	}
}

func (g *fatGrouping) Join(sub Subject, role Role) error {
	if e := g.Grouping.Join(sub, role); e != nil {
		return e
	}

	if g.roles[sub] == nil {
		g.roles[sub] = make(map[Role]struct{})
	}
	g.roles[sub][role] = struct{}{}
	for r := range g.roles[role] {
		g.roles[sub][r] = struct{}{}
	}

	if g.users[role] == nil {
		g.users[role] = make(map[User]struct{})
	}
	if su, ok := sub.(User); ok {
		g.users[role][su] = struct{}{}
	} else {
		for u := range g.users[sub.(Role)] {
			g.users[role][u] = struct{}{}
		}
	}

	return nil
}

func (g *fatGrouping) Leave(sub Subject, role Role) error {
	if e := g.Grouping.Leave(sub, role); e != nil {
		return e
	}

	if e := g.rebuildRoles(sub); e != nil {
		return e
	}
	if e := g.rebuildUsers(role); e != nil {
		return e
	}
	return nil
}

func (g *fatGrouping) HasRole(user User, role Role) (bool, error) {
	_, ok := g.users[role]
	return ok, nil
}

func (g *fatGrouping) RolesOf(user User) (map[Role]struct{}, error) {
	return g.roles[user], nil
}

func (g *fatGrouping) UsersOf(role Role) (map[User]struct{}, error) {
	return g.users[role], nil
}

func (g *fatGrouping) RemoveRole(role Role) error {
	subs, e := g.Grouping.DirectSubjectsOf(role)
	if e != nil {
		return e
	}
	roles, e := g.Grouping.DirectRolesOf(role)
	if e != nil {
		return e
	}

	if e := g.Grouping.RemoveRole(role); e != nil {
		return e
	}

	for sub := range subs {
		if e := g.rebuildRoles(sub); e != nil {
			return e
		}
	}
	for role := range roles {
		if e := g.rebuildUsers(role); e != nil {
			return e
		}
	}

	return nil
}

func (g *fatGrouping) RemoveUser(user User) error {
	roles, e := g.Grouping.DirectRolesOf(user)
	if e != nil {
		return e
	}

	if e := g.Grouping.RemoveUser(user); e != nil {
		return e
	}

	for role := range roles {
		if e := g.rebuildUsers(role); e != nil {
			return e
		}
	}

	return nil
}

func (g *fatGrouping) rebuildRoles(sub Subject) error {
	// rebuild roles for subject
	roles, e := g.Grouping.DirectRolesOf(sub)
	if e != nil {
		return e
	}
	g.roles[sub] = make(map[Role]struct{}, len(roles))
	for role := range roles {
		g.roles[sub][role] = struct{}{}
		for rr := range g.roles[role] {
			g.roles[sub][rr] = struct{}{}
		}
	}

	// rebuild roles for all roles of role
	subs, e := g.Grouping.DirectRolesOf(sub)
	if e != nil {
		return e
	}
	for sub := range subs {
		if e := g.rebuildRoles(sub); e != nil {
			return e
		}
	}

	return nil
}

func (g *fatGrouping) rebuildUsers(role Role) error {
	// rebuild users of role
	subs, e := g.Grouping.DirectSubjectsOf(role)
	if e != nil {
		return e
	}

	g.users[role] = make(map[User]struct{}, len(subs))
	for sub := range subs {
		if user, ok := sub.(User); ok {
			g.users[role][user] = struct{}{}
		} else {
			for user := range g.users[sub.(Role)] {
				g.users[role][user] = struct{}{}
			}
		}
	}

	// rebuild users for all roles of role
	roles, e := g.Grouping.DirectRolesOf(role)
	if e != nil {
		return e
	}
	for role := range roles {
		if e := g.rebuildUsers(role); e != nil {
			return e
		}
	}

	return nil
}
