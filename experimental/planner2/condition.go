package planner2

import "strings"

type IdentityCondition struct{}

func (IdentityCondition) Satisfies(name PackageName) (MatchResult, error) {
	return MatchResultMatched, nil
}

func (IdentityCondition) Key() string { return "identity" }

type AndCondition []Condition

func (a AndCondition) Satisfies(name PackageName) (MatchResult, error) {
	for _, cond := range a {
		match, err := cond.Satisfies(name)
		if err != nil {
			return MatchResultNoMatch, err
		}

		if match != MatchResultMatched {
			return MatchResultNoMatch, nil
		}
	}

	return MatchResultMatched, nil
}

func (a AndCondition) Key() string {
	var ret []string

	for _, cond := range a {
		ret = append(ret, cond.Key())
	}

	return "and(" + strings.Join(ret, ",") + ")"
}

func (a AndCondition) String() string { return a.Key() }

type OrCondition []Condition

func (a OrCondition) Satisfies(name PackageName) (MatchResult, error) {
	for _, cond := range a {
		match, err := cond.Satisfies(name)
		if err != nil {
			return MatchResultNoMatch, err
		}

		if match == MatchResultMatched {
			return MatchResultMatched, nil
		}
	}

	return MatchResultNoMatch, nil
}

func (a OrCondition) Key() string {
	var ret []string

	for _, cond := range a {
		ret = append(ret, cond.Key())
	}

	return "or(" + strings.Join(ret, ",") + ")"
}

func (a OrCondition) String() string { return a.Key() }

type NotCondition struct {
	Condition Condition
}

func (a NotCondition) Satisfies(name PackageName) (MatchResult, error) {
	match, err := a.Condition.Satisfies(name)
	if err != nil {
		return MatchResultNoMatch, err
	}

	if match == MatchResultMatched {
		return MatchResultNoMatch, nil
	}

	return MatchResultMatched, nil
}

func (a NotCondition) Key() string {
	return "not(" + a.Condition.Key() + ")"
}

func (a NotCondition) String() string { return a.Key() }

var (
	_ Condition = IdentityCondition{}
	_ Condition = AndCondition{}
	_ Condition = OrCondition{}
	_ Condition = NotCondition{}
)

func CombineConditions(a Condition, b Condition) Condition {
	if a.Key() == b.Key() {
		return a
	}

	if a.Key() == "identity" {
		return b
	} else if b.Key() == "identity" {
		return a
	}

	switch a.(type) {
	case AndCondition:
		switch b.(type) {
		case AndCondition:
			return append(a.(AndCondition), b.(AndCondition)...)
		default:
			return append(a.(AndCondition), b)
		}
	case OrCondition:
		switch b.(type) {
		case OrCondition:
			return append(a.(OrCondition), b.(OrCondition)...)
		default:
			return append(a.(OrCondition), b)
		}
	default:
		return AndCondition{a, b}
	}
}
