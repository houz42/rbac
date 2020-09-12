package types

import "strings"

// Action can be done on objects by subjects
// Actions are power of twos to achieve efficient set operations, like union, intersection, complemention
type Action uint32

// preset actions, users can reset these and define others
const (
	Exec Action = 1 << iota
	Write
	Read

	None          Action = 0
	ReadWrite            = Read | Write
	ReadExec             = Read | Exec
	ReadWriteExec        = Read | Write | Exec
)

// AllActions is union of all actions, it will be reset when ResetActions being called
var AllActions = ReadWriteExec

var actionNames = map[Action]string{
	Read:  "read",
	Write: "write",
	Exec:  "exec",
}

func ResetActions(names ...string) []Action {
	actionNames = make(map[Action]string)
	actions := make([]Action, 0, len(names))
	AllActions = 0

	for i, name := range names {
		a := Action(1 << i)
		actionNames[a] = name
		actions = append(actions, a)
		AllActions |= a
	}

	return actions
}

func (a Action) IsIn(b Action) bool {
	return a|b == b
}

func (a Action) Includes(b Action) bool {
	return b.IsIn(a)
}

func (a Action) Difference(b Action) Action {
	return a &^ b
}

// Split a union of actions to slice of single actions
func (a Action) Split() []Action {
	out := make([]Action, 0)
	op := Action(1)
	for op <= a {
		if op&a > 0 {
			out = append(out, op)
		}
		op <<= 1
	}
	return out
}

func (a Action) String() string {
	as := a.Split()
	ns := make([]string, 0, len(as))
	for _, a := range as {
		n, ok := actionNames[a]
		if !ok {
			n = "unknown"
		}
		ns = append(ns, n)
	}
	return strings.Join(ns, "|")
}