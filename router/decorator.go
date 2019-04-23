package router

import (
	"sort"

	"github.com/gammazero/nexus/wamp"
)

type decoratorMap struct {
	prefixMatch   map[wamp.URI][]*Decorator
	wildcardMatch map[wamp.URI][]*Decorator
	exactMatch    map[wamp.URI][]*Decorator
}

func (dm *decoratorMap) matchDecorators(procedure wamp.URI) []*Decorator {
	decorators := []*Decorator{}

	exactList := dm.exactMatch[procedure]
	decorators = append(decorators, exactList...)
	for pfxURI, pfxDecList := range dm.prefixMatch {
		if !procedure.PrefixMatch(pfxURI) {
			continue
		}
		decorators = append(decorators, pfxDecList...)
	}
	for wcURI, wcDecList := range dm.wildcardMatch {
		if !procedure.WildcardMatch(wcURI) {
			continue
		}
		decorators = append(decorators, wcDecList...)
	}
	sort.Slice(decorators, func(i, j int) bool {
		return decorators[i].order < decorators[j].order
	})
	return decorators
}

func newDecoratorMap() *decoratorMap {
	return &decoratorMap{
		prefixMatch:   make(map[wamp.URI][]*Decorator),
		wildcardMatch: make(map[wamp.URI][]*Decorator),
		exactMatch:    make(map[wamp.URI][]*Decorator),
	}
}

type Decorator struct {
	handlerURI wamp.URI
	order      int64
	callType   wamp.DecoratorCallType
	id         wamp.ID
	owner      wamp.ID
}

func (r *realm) NewDecorator(
	handlerURI wamp.URI,
	order int64,
	callType wamp.DecoratorCallType,
	sid wamp.ID,
) (*Decorator, wamp.URI) {

	// check whether the handler is a valid and registered procedure.
	_, hasRegistration := r.dealer.matchProcedure(handlerURI)
	if !hasRegistration {
		return nil, wamp.ErrNoSuchProcedure
	}

	createdDecorator := Decorator{
		handlerURI,
		order,
		callType,
		wamp.GlobalID(),
		sid,
	}

	return &createdDecorator, ""
}

func (r *realm) AddDecoratorHandler(msg *wamp.Invocation) wamp.Message {

	r.log.Print("AddDecoratorHandler called")

	decoratorKind, isOk := wamp.AsString(msg.Arguments[0])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}

	sid, isOk := wamp.AsID(msg.Details["caller"])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}

	var dm *decoratorMap
	var syncChan chan func()
	switch wamp.DecoratorType(decoratorKind) {
	case wamp.DecoratorTypePreprocess:
		dm = r.dealer.preprocessDecorators
		syncChan = r.dealer.actionChan
	case wamp.DecoratorTypePrecall:
		dm = r.dealer.precallDecorators
		syncChan = r.dealer.actionChan
	case wamp.DecoratorTypePostcall:
		dm = r.dealer.postcallDecorators
		syncChan = r.dealer.actionChan

	case wamp.DecoratorTypePublish:
		dm = r.broker.publishDecorators
		syncChan = r.broker.actionChan
	case wamp.DecoratorTypeEvent:
		dm = r.broker.eventDecorators
		syncChan = r.broker.actionChan
	default:
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}

	handlerURI, isOk := wamp.AsURI(msg.Arguments[3])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}
	order, isOk := wamp.AsInt64(msg.Arguments[4])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}

	callTypeString, isOk := wamp.AsString(msg.Arguments[5])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}
	var callType wamp.DecoratorCallType
	switch wamp.DecoratorCallType(callTypeString) {
	case "sync":
		callType = wamp.DecoratorCallTypeSync
	case "async":
		callType = wamp.DecoratorCallTypeAsync
	default:
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}

	createdDecorator, errURI := r.NewDecorator(handlerURI, order, callType, sid)

	// TODO: Think about empty string as empty wamp uri.
	if errURI != "" {
		return makeError(msg.Request, errURI)
	}

	matchURI, isOk := wamp.AsURI(msg.Arguments[1])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}
	matchType, isOk := wamp.AsString(msg.Arguments[2])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}
	done := make(chan bool)
	syncChan <- func() {
		var target map[wamp.URI][]*Decorator
		switch matchType {
		case wamp.MatchPrefix:
			target = dm.prefixMatch
		case wamp.MatchWildcard:
			target = dm.wildcardMatch
		default:
			target = dm.exactMatch
		}

		list := target[matchURI]
		list = append(list, createdDecorator)
		target[matchURI] = list
		done <- true
	}
	<-done
	r.decorators[createdDecorator.id] = decoratorBucket{
		owner:    sid,
		kind:     wamp.DecoratorType(decoratorKind),
		match:    matchType,
		matchURI: matchURI,
	}

	r.log.Printf("Created and regstered decorator with ID %v", createdDecorator.id)
	return &wamp.Yield{Request: msg.Request, Arguments: wamp.List{createdDecorator.id}}
}

func (r *realm) RemoveDecoratorHandler(msg *wamp.Invocation) wamp.Message {
	decoratorID, isOk := wamp.AsID(msg.Arguments[0])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}
	ownerID, isOk := wamp.AsID(msg.Details["caller"])
	if !isOk {
		return makeError(msg.Request, wamp.ErrInvalidArgument)
	}

	decorator, ok := r.decorators[decoratorID]
	if !ok {
		return makeError(msg.Request, wamp.ErrNoSuchDecorator)
	}

	if decorator.owner != ownerID {
		return makeError(msg.Request, wamp.ErrNotAuthorized)
	}

	r.log.Printf("Removing Decorator with ID %v", decoratorID)
	delete(r.decorators, decoratorID)

	var syncChan chan func()
	var targetMap *decoratorMap
	doneChan := make(chan bool)
	switch decorator.kind {
	case wamp.DecoratorTypeEvent:
		syncChan = r.broker.actionChan
		targetMap = r.broker.eventDecorators
	case wamp.DecoratorTypePublish:
		syncChan = r.broker.actionChan
		targetMap = r.broker.publishDecorators
	case wamp.DecoratorTypePreprocess:
		syncChan = r.dealer.actionChan
		targetMap = r.dealer.preprocessDecorators
	case wamp.DecoratorTypePrecall:
		syncChan = r.dealer.actionChan
		targetMap = r.dealer.precallDecorators
	case wamp.DecoratorTypePostcall:
		syncChan = r.dealer.actionChan
		targetMap = r.dealer.postcallDecorators
	}

	syncChan <- func() {
		var decoratorLookup map[wamp.URI][]*Decorator
		switch decorator.match {
		case wamp.MatchExact:
			decoratorLookup = targetMap.exactMatch
		case wamp.MatchPrefix:
			decoratorLookup = targetMap.prefixMatch
		case wamp.MatchWildcard:
			decoratorLookup = targetMap.wildcardMatch
		}
		decoratorList := decoratorLookup[decorator.matchURI]
		for i, storedDecorator := range decoratorList {
			if storedDecorator.id == decoratorID {
				decoratorList = append(decoratorList[:i], decoratorList[i+1:]...)
				break
			}
		}

		if len(decoratorList) == 0 {
			delete(decoratorLookup, decorator.matchURI)
		} else {
			decoratorLookup[decorator.matchURI] = decoratorList
		}
		doneChan <- true
	}
	<-doneChan

	return &wamp.Yield{Request: msg.Request}
}
