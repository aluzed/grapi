package query

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/schema"
)

// Event is a row lifecycle event, as Strapi's db.lifecycles builds it.
// Params may be changed by the before* subscribers; State is kept between
// the before and the after of one operation, per subscriber.
type Event struct {
	Ctx    context.Context
	Action string
	Model  *metadata.Model
	Params *EventParams
	State  map[string]any
	// Result is set for the after* events.
	Result any
}

// EventParams are the operation's params.
type EventParams struct {
	Data     any
	Where    any
	Select   []string
	Populate any
	OrderBy  any
	Limit    *int
	Offset   *int
	Filters  any
}

// Subscriber receives the events of Models (all models when empty). Hooks
// maps an action ("beforeCreate", "afterUpdate"...) to its handler; All, when
// set, gets every event.
type Subscriber struct {
	Models []string
	Hooks  map[string]func(*Event) error
	All    func(*Event) error
}

// Lifecycles is db.lifecycles.
type Lifecycles struct {
	mu   sync.RWMutex
	subs []*Subscriber
}

// Subscribe adds s and returns the function that removes it.
func (l *Lifecycles) Subscribe(s Subscriber) func() {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := &s
	l.subs = append(l.subs, p)
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		for i, e := range l.subs {
			if e == p {
				l.subs = append(l.subs[:i:i], l.subs[i+1:]...)
				return
			}
		}
	}
}

type states map[*Subscriber]map[string]any

func (l *Lifecycles) run(ctx context.Context, action string, m *metadata.Model, params *EventParams, result any, st states) (states, error) {
	if l == nil {
		return nil, nil
	}
	l.mu.RLock()
	subs := slices.Clone(l.subs)
	l.mu.RUnlock()
	if st == nil {
		st = states{}
	}
	for _, s := range subs {
		if len(s.Models) > 0 && !slices.Contains(s.Models, m.UID) {
			continue
		}
		fn := s.Hooks[action]
		if fn == nil {
			fn = s.All
		}
		if fn == nil {
			continue
		}
		state, ok := st[s]
		if !ok {
			state = map[string]any{}
			st[s] = state
		}
		e := &Event{Ctx: ctx, Action: action, Model: m, Params: params, State: state}
		if strings.HasPrefix(action, "after") {
			e.Result = result
		}
		if err := fn(e); err != nil {
			return st, err
		}
	}
	return st, nil
}

func (e *Engine) model(uid string) *metadata.Model {
	m, _ := e.MD.Model(uid)
	if m == nil {
		m = &metadata.Model{UID: uid}
	}
	return m
}

func queryParams(p Params) *EventParams {
	return &EventParams{Where: p.Where, Select: p.Select, Populate: p.Populate, OrderBy: p.OrderBy, Limit: p.Limit, Offset: p.Offset, Filters: p.Filters}
}

func (ep *EventParams) query(p Params) Params {
	p.Where, p.Select, p.Populate, p.OrderBy, p.Limit, p.Offset, p.Filters = ep.Where, ep.Select, ep.Populate, ep.OrderBy, ep.Limit, ep.Offset, ep.Filters
	return p
}

// timestamps is Strapi's built-in subscriber, run before the others:
// createdAt and updatedAt when missing on create, updatedAt on update.
func timestamps(m *metadata.Model, data map[string]any, create bool) {
	if data == nil {
		return
	}
	now := time.Now()
	if _, ok := m.Attributes.Get(schema.AttrCreatedAt); ok && create {
		setDefault(data, schema.AttrCreatedAt, now)
	}
	if _, ok := m.Attributes.Get(schema.AttrUpdatedAt); ok {
		if create {
			setDefault(data, schema.AttrUpdatedAt, now)
		} else {
			data[schema.AttrUpdatedAt] = now
		}
	}
}

// FindMany returns the rows matching p.
func (e *Engine) FindMany(ctx context.Context, uid string, p Params) ([]Row, error) {
	m, ep := e.model(uid), queryParams(p)
	st, err := e.Lifecycles.run(ctx, "beforeFindMany", m, ep, nil, nil)
	if err != nil {
		return nil, err
	}
	rows, err := e.findManyCore(ctx, uid, ep.query(p))
	if err != nil {
		return nil, err
	}
	_, err = e.Lifecycles.run(ctx, "afterFindMany", m, ep, rows, st)
	return rows, err
}

// FindOne returns the first row matching p, or nil.
func (e *Engine) FindOne(ctx context.Context, uid string, p Params) (Row, error) {
	m, ep := e.model(uid), queryParams(p)
	st, err := e.Lifecycles.run(ctx, "beforeFindOne", m, ep, nil, nil)
	if err != nil {
		return nil, err
	}
	row, err := e.findOneCore(ctx, uid, ep.query(p))
	if err != nil {
		return nil, err
	}
	_, err = e.Lifecycles.run(ctx, "afterFindOne", m, ep, row, st)
	return row, err
}

// Count counts the entries matching p.
func (e *Engine) Count(ctx context.Context, uid string, p Params) (int64, error) {
	m, ep := e.model(uid), queryParams(p)
	st, err := e.Lifecycles.run(ctx, "beforeCount", m, ep, nil, nil)
	if err != nil {
		return 0, err
	}
	n, err := e.countCore(ctx, uid, ep.query(p))
	if err != nil {
		return 0, err
	}
	_, err = e.Lifecycles.run(ctx, "afterCount", m, ep, n, st)
	return n, err
}

// Create inserts an entry and attaches its relations, then reads it back.
func (e *Engine) Create(ctx context.Context, uid string, data Data, p Params) (Row, error) {
	m := e.model(uid)
	if data != nil {
		data = cloneData(data)
		timestamps(m, data, true)
	}
	ep := &EventParams{Data: data, Select: p.Select, Populate: p.Populate}
	st, err := e.Lifecycles.run(ctx, "beforeCreate", m, ep, nil, nil)
	if err != nil {
		return nil, err
	}
	d, _ := ep.Data.(Data)
	if dm, ok := ep.Data.(map[string]any); ok {
		d = dm
	}
	p.Select, p.Populate = ep.Select, ep.Populate
	row, err := e.createCore(ctx, uid, d, p)
	if err != nil {
		return nil, err
	}
	_, err = e.Lifecycles.run(ctx, "afterCreate", m, ep, row, st)
	return row, err
}

// CreateMany inserts rows without relations and returns their ids.
func (e *Engine) CreateMany(ctx context.Context, uid string, data []Data) (int, []any, error) {
	m := e.model(uid)
	for i, d := range data {
		data[i] = cloneData(d)
		timestamps(m, data[i], true)
	}
	ep := &EventParams{Data: data}
	st, err := e.Lifecycles.run(ctx, "beforeCreateMany", m, ep, nil, nil)
	if err != nil {
		return 0, nil, err
	}
	if d, ok := ep.Data.([]Data); ok {
		data = d
	}
	n, ids, err := e.createManyCore(ctx, uid, data)
	if err != nil {
		return 0, nil, err
	}
	_, err = e.Lifecycles.run(ctx, "afterCreateMany", m, ep, map[string]any{"count": n, "ids": ids}, st)
	return n, ids, err
}

// Update changes the first entry matching where and its relations.
func (e *Engine) Update(ctx context.Context, uid string, where any, data Data, p Params) (Row, error) {
	m := e.model(uid)
	if data != nil {
		data = cloneData(data)
		timestamps(m, data, false)
	}
	ep := &EventParams{Data: data, Where: where, Select: p.Select, Populate: p.Populate}
	st, err := e.Lifecycles.run(ctx, "beforeUpdate", m, ep, nil, nil)
	if err != nil {
		return nil, err
	}
	d, _ := ep.Data.(Data)
	if dm, ok := ep.Data.(map[string]any); ok {
		d = dm
	}
	p.Select, p.Populate = ep.Select, ep.Populate
	row, err := e.updateCore(ctx, uid, ep.Where, d, p)
	if err != nil {
		return nil, err
	}
	_, err = e.Lifecycles.run(ctx, "afterUpdate", m, ep, row, st)
	return row, err
}

// UpdateMany changes every entry matching p.
func (e *Engine) UpdateMany(ctx context.Context, uid string, p Params, data Data) (int64, error) {
	m := e.model(uid)
	if data != nil {
		data = cloneData(data)
		timestamps(m, data, false)
	}
	ep := &EventParams{Data: data, Where: p.Where}
	st, err := e.Lifecycles.run(ctx, "beforeUpdateMany", m, ep, nil, nil)
	if err != nil {
		return 0, err
	}
	if dm, ok := ep.Data.(map[string]any); ok {
		data = dm
	}
	p.Where = ep.Where
	n, err := e.updateManyCore(ctx, uid, p, data)
	if err != nil {
		return 0, err
	}
	_, err = e.Lifecycles.run(ctx, "afterUpdateMany", m, ep, map[string]any{"count": n}, st)
	return n, err
}

// Delete removes the first entry matching where and its relation links.
func (e *Engine) Delete(ctx context.Context, uid string, where any, p Params) (Row, error) {
	m, ep := e.model(uid), &EventParams{Where: where, Select: p.Select, Populate: p.Populate}
	st, err := e.Lifecycles.run(ctx, "beforeDelete", m, ep, nil, nil)
	if err != nil {
		return nil, err
	}
	p.Select, p.Populate = ep.Select, ep.Populate
	row, err := e.deleteCore(ctx, uid, ep.Where, p)
	if err != nil {
		return nil, err
	}
	_, err = e.Lifecycles.run(ctx, "afterDelete", m, ep, row, st)
	return row, err
}

// DeleteMany removes every entry matching p.
func (e *Engine) DeleteMany(ctx context.Context, uid string, p Params) (int64, error) {
	m, ep := e.model(uid), &EventParams{Where: p.Where}
	st, err := e.Lifecycles.run(ctx, "beforeDeleteMany", m, ep, nil, nil)
	if err != nil {
		return 0, err
	}
	p.Where = ep.Where
	n, err := e.deleteManyCore(ctx, uid, p)
	if err != nil {
		return 0, err
	}
	_, err = e.Lifecycles.run(ctx, "afterDeleteMany", m, ep, map[string]any{"count": n}, st)
	return n, err
}
