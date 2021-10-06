package shred

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/phomola/rbtree"
)

// Symbol is a terminal or non-terminal.
type Symbol interface {
	fmt.Stringer
}

// NonTerminal is a non-terminal symbol.
type NonTerminal struct {
	Name string
}

// String returns the name of the non-terminal.
func (s NonTerminal) String() string { return s.Name }

// Terminal is a terminal symbol.
type Terminal interface {
	Symbol
	Kind() Kind
}

// An EOF terminal.
type EOF struct{}

func (s EOF) String() string { return "_eof_" }

func (s EOF) Kind() Kind { return KindEOF }

// An identifier terminal.
type Ident struct{}

func (s Ident) String() string { return "_ident_" }

func (s Ident) Kind() Kind { return KindIdent }

// A match terminal.
type Match struct {
	Text string
}

func (s Match) String() string { return fmt.Sprintf(`"%s"`, s.Text) }

func (s Match) Kind() Kind { return KindMatch }

func terminalFromToken(tok Token) (Terminal, bool) {
	switch {
	case tok.IsIdent():
		return Match{tok.Text()}, true
	case tok.IsEOF():
		return EOF{}, false
	case tok.Kind() == KindOther:
		return Match{tok.Text()}, false
	}
	panic("couldn't convert token " + tok.String() + " to terminal")
}

// A context-free rule with an assiciated AST builder.
type Rule struct {
	Lhs     string
	Rhs     []Symbol
	Builder func([]interface{}) interface{}
}

func (r *Rule) String() string {
	ret := r.Lhs + " ->"
	for _, s := range r.Rhs {
		ret += " " + s.String()
	}
	return ret
}

func (r *Rule) stringWithDot(pos int) string {
	ret := r.Lhs + " ->"
	for i, s := range r.Rhs {
		if i == pos {
			ret += " ."
		}
		ret += " " + s.String()
	}
	if pos == len(r.Rhs) {
		ret += " ."
	}
	return ret
}

type item struct {
	rule int
	dot  int
}

func (i1 item) less(i2 item) bool {
	switch {
	case i1.rule < i2.rule:
		return true
	case i1.rule > i2.rule:
		return false
	}
	return i1.dot < i2.dot
}

type state struct {
	items []item
}

func (s *state) addItem(it item) bool {
	for _, el := range s.items {
		if el == it {
			return false
		}
	}
	s.items = append(s.items, it)
	sort.Slice(s.items, func(i, j int) bool {
		i1, i2 := s.items[i], s.items[j]
		return i1.less(i2)
	})
	return true
}

func (s1 *state) Compare(s2 interface{}) int {
	if s2, ok := s2.(*state); ok {
		switch {
		case len(s1.items) < len(s2.items):
			return -1
		case len(s1.items) > len(s2.items):
			return 1
		}
		for i, i1 := range s1.items {
			i2 := s2.items[i]
			switch {
			case i1.less(i2):
				return -1
			case i2.less(i1):
				return 1
			}
		}
		return 0
	}
	panic("Compare argument is not a state pointer")
}

type action interface{}

type stop struct{}

type shift struct{ state *state }

type reduce struct{ rule *Rule }

// An attribute LR-grammar.
type Grammar struct {
	Rules        []*Rule
	actions      *rbtree.Tree
	gotos        *rbtree.Tree
	nonterminals map[NonTerminal]struct{}
	terminals    map[Terminal]struct{}
	initState    *state
}

// NewGrammar creates a new grammar with the given rules.
func NewGrammar(rules []*Rule) *Grammar {
	return &Grammar{
		Rules:        rules,
		actions:      rbtree.New(),
		gotos:        rbtree.New(),
		nonterminals: make(map[NonTerminal]struct{}),
		terminals:    make(map[Terminal]struct{})}
}

// func (gr *Grammar) Automaton() {
// 	keys := gr.actions.Keys()
// 	states := make(map[int]*state)
// 	nums := rbtree.New()
// 	var initState int
// 	for i, k := range keys {
// 		s := k.(*state)
// 		states[i] = s
// 		nums.Insert(s, i)
// 		if s == gr.initState {
// 			initState = i
// 		}
// 	}
// }

func (gr *Grammar) itemAsString(it item) string {
	r := gr.Rules[it.rule]
	return r.stringWithDot(it.dot)
}

func (gr *Grammar) stateAsString(s *state) string {
	ret := make([]string, len(s.items))
	for i, it := range s.items {
		ret[i] = gr.itemAsString(it)
	}
	return strings.Join(ret, " + ")
}

func (gr *Grammar) rulesWithLhs(lhs string) []int {
	var ret []int
	for i, r := range gr.Rules {
		if r.Lhs == lhs {
			ret = append(ret, i)
		}
	}
	return ret
}

func (gr *Grammar) stateNonTerminals(s *state) map[NonTerminal]*state {
	m := make(map[NonTerminal]*state)
	for _, it := range s.items {
		r := gr.Rules[it.rule]
		if it.dot < len(r.Rhs) {
			if nt, ok := r.Rhs[it.dot].(NonTerminal); ok {
				s := m[nt]
				if s == nil {
					s = new(state)
					m[nt] = s
				}
				s.addItem(item{it.rule, it.dot + 1})
			}
		}
	}
	for _, s := range m {
		gr.closeState(s)
	}
	return m
}

func (gr *Grammar) stateTerminals(s *state) map[Terminal]*state {
	m := make(map[Terminal]*state)
	for _, it := range s.items {
		r := gr.Rules[it.rule]
		if it.dot < len(r.Rhs) {
			if t, ok := r.Rhs[it.dot].(Terminal); ok {
				s := m[t]
				if s == nil {
					s = new(state)
					m[t] = s
				}
				s.addItem(item{it.rule, it.dot + 1})
			}
		}
	}
	for _, s := range m {
		gr.closeState(s)
	}
	return m
}

func (gr *Grammar) reductions(s *state) []*Rule {
	var ret []*Rule
	for _, it := range s.items {
		r := gr.Rules[it.rule]
		if it.dot == len(r.Rhs) {
			ret = append(ret, r)
		}
	}
	return ret
}

func (gr *Grammar) closeState(s *state) {
	processed := make(map[item]struct{})
	for {
		var newItems []item
		for _, it := range s.items {
			if _, ok := processed[it]; ok {
				continue
			}
			processed[it] = struct{}{}
			r := gr.Rules[it.rule]
			if it.dot < len(r.Rhs) {
				if nt, ok := r.Rhs[it.dot].(NonTerminal); ok {
					for _, r := range gr.rulesWithLhs(nt.Name) {
						newItems = append(newItems, item{r, 0})
					}
				}
			}
		}
		added := false
		for _, it := range newItems {
			if s.addItem(it) {
				added = true
			}
		}
		if !added {
			break
		}
	}
}

func (gr *Grammar) addState(s *state) error {
	if _, ok := gr.actions.Get(s); ok {
		return nil
	}
	// fmt.Println("new state:", gr.stateAsString(s))
	a := make(map[Terminal]action)
	gr.actions.Insert(s, a)
	rs := gr.reductions(s)
	if len(rs) > 1 {
		return errors.New("too many reductions for state " + gr.stateAsString(s))
	}
	if len(rs) == 1 {
		r := rs[0]
		// fmt.Println("reduction:", r)
		for t := range gr.terminals {
			a[t] = reduce{r}
		}
	}
	for t, s2 := range gr.stateTerminals(s) {
		// fmt.Println("shift:", t, "=>", gr.stateAsString(s2))
		if act, ok := a[t]; ok {
			if _, ok := act.(shift); ok {
				return errors.New("multiples shifts over '" + t.String() + "' for " + gr.stateAsString(s))
			}
		}
		a[t] = shift{s2}
		gr.addState(s2)
	}
	g := make(map[NonTerminal]*state)
	gr.gotos.Insert(s, g)
	for nt, s2 := range gr.stateNonTerminals(s) {
		// fmt.Println("goto:", nt, "=>", gr.stateAsString(s2))
		g[nt] = s2
		gr.addState(s2)
	}
	return nil
}

// Build builds an automaton for the grammar.
func (gr *Grammar) Build() error {
	for _, r := range gr.Rules {
		for _, s := range r.Rhs {
			switch s := s.(type) {
			case NonTerminal:
				gr.nonterminals[s] = struct{}{}
			case Terminal:
				gr.terminals[s] = struct{}{}
			}
		}
	}
	gr.terminals[EOF{}] = struct{}{}
	s := new(state)
	for _, r := range gr.rulesWithLhs("0") {
		s.items = append(s.items, item{r, 0})
	}
	gr.closeState(s)
	gr.initState = s
	err := gr.addState(s)
	if err != nil {
		return err
	}
	return nil
}

// Parse parses a sequence of tokens.
func (gr *Grammar) Parse(tokens []Token) (interface{}, error) {
	var stack []interface{}
	st, i := gr.initState, 0
	states := []*state{st}
	for {
		tok := tokens[i]
		a, ok := gr.actions.Get(st)
		if !ok {
			return nil, errors.New("no actions for state " + gr.stateAsString(st))
		}
		as := a.(map[Terminal]action)
		t, id := terminalFromToken(tok)
		act, ok := as[t]
		if !ok && id {
			act, ok = as[Ident{}]
		}
		if !ok {
			return nil, errors.New("no action over '" + t.String() + "' for state " + gr.stateAsString(st))
		}
		switch act := act.(type) {
		case stop:
			return stack[len(stack)-1], nil
		case shift:
			stack = append(stack, tok)
			st = act.state
			states = append(states, st)
			i++
		case reduce:
			r := act.rule
			l := len(r.Rhs)
			data := stack[len(stack)-l:]
			stack = append(stack[:len(stack)-l], r.Builder(data))
			if r.Lhs == "0" {
				if len(stack) != 1 {
					panic("corrupted symbol stack")
				}
				return stack[len(stack)-1], nil
			}
			states = states[:len(states)-l]
			pst := states[len(states)-1]
			g, ok := gr.gotos.Get(pst)
			if !ok {
				return nil, errors.New("no gotos for state " + gr.stateAsString(st))
			}
			gt := g.(map[NonTerminal]*state)
			st2, ok := gt[NonTerminal{r.Lhs}]
			if !ok {
				return nil, errors.New("no goto over '" + r.Lhs + "' for state " + gr.stateAsString(st))
			}
			st = st2
			states = append(states, st)
		default:
			panic("unknown action")
		}
	}
}
