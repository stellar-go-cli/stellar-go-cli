package soroban

import (
	"context"
	"fmt"

	rpc "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// Topic wildcard constants for event filters, as defined by the getEvents
// RPC spec: "*" matches exactly one topic segment, "**" (last segment only)
// matches zero or more trailing segments.
const (
	TopicWildcardOne  = "*"
	TopicWildcardRest = "**"
)

// maxEventsLimit is the upper bound on getEvents page size, matching the
// limit Soroban RPC servers enforce.
const maxEventsLimit = 10_000

// EventTopicSegment is one segment of an event topic filter. Use TopicVal,
// TopicSymbol, TopicAny or TopicAnyMore to build segments.
type EventTopicSegment struct {
	// Wildcard is "*" or "**". When set, Value is ignored.
	Wildcard string
	// Value matches the topic segment exactly.
	Value *xdr.ScVal
}

// TopicVal matches an exact ScVal topic segment.
func TopicVal(v xdr.ScVal) EventTopicSegment {
	return EventTopicSegment{Value: &v}
}

// TopicSymbol matches a symbol topic segment — the usual form of contract
// event names, e.g. TopicSymbol("commit").
func TopicSymbol(name string) EventTopicSegment {
	return TopicVal(ScvSymbol(name))
}

// TopicAny matches exactly one topic segment ("*").
func TopicAny() EventTopicSegment {
	return EventTopicSegment{Wildcard: TopicWildcardOne}
}

// TopicAnyMore matches zero or more trailing topic segments ("**"); it is
// only valid as the last segment of a topic filter.
func TopicAnyMore() EventTopicSegment {
	return EventTopicSegment{Wildcard: TopicWildcardRest}
}

// EventFilter selects events by type, emitting contract and topic pattern.
// At most 5 filters are allowed per request; each filter may carry up to 5
// contract IDs and 5 alternative topic patterns.
type EventFilter struct {
	// Types limits the event types matched, e.g. "contract" or "system".
	// Empty matches all types.
	Types []string
	// ContractIDs limits events to the given contract addresses (C...).
	ContractIDs []string
	// Topics lists alternative topic patterns; an event matches when any of
	// the patterns match. Each pattern is a sequence of EventTopicSegment.
	Topics [][]EventTopicSegment
}

// EventsRequest is the parameter set for Client.GetEvents.
type EventsRequest struct {
	// StartLedger is the first ledger to search (required unless Cursor is set).
	StartLedger uint32
	// EndLedger optionally bounds the search; 0 means no upper bound.
	EndLedger uint32
	// Filters select which events to return (max 5).
	Filters []EventFilter
	// Limit caps the number of events per page.
	Limit uint
	// Cursor resumes a previous query; pass EventsResult.Cursor. When set,
	// StartLedger and EndLedger must be zero.
	Cursor string
}

// ContractEvent is a decoded contract event: topics and value as xdr.ScVal.
type ContractEvent struct {
	Type           string
	Ledger         uint32
	LedgerClosedAt string
	ContractID     string
	// ID is the event's cursor position within the ledger stream.
	ID      string
	TxHash  string
	TxIndex uint32
	OpIndex uint32
	Topics  []xdr.ScVal
	Value   xdr.ScVal
}

// EventsResult is a page of events plus the cursor for the next page.
type EventsResult struct {
	Events       []ContractEvent
	Cursor       string
	LatestLedger uint32
	OldestLedger uint32
}

// toRPC converts the request into the SDK wire type and validates it.
func (r *EventsRequest) toRPC() (rpc.GetEventsRequest, error) {
	req := rpc.GetEventsRequest{
		StartLedger: r.StartLedger,
		EndLedger:   r.EndLedger,
	}

	if r.Limit > 0 || r.Cursor != "" {
		req.Pagination = &rpc.PaginationOptions{Limit: r.Limit}
		if r.Cursor != "" {
			cursor, err := rpc.ParseCursor(r.Cursor)
			if err != nil {
				return req, fmt.Errorf("invalid events cursor: %w", err)
			}
			req.Pagination.Cursor = &cursor
		}
	}

	for _, f := range r.Filters {
		rf := rpc.EventFilter{ContractIDs: f.ContractIDs}
		if len(f.Types) > 0 {
			rf.EventType = rpc.EventTypeSet{}
			for _, t := range f.Types {
				rf.EventType[t] = nil
			}
		}
		for _, pattern := range f.Topics {
			if len(pattern) == 0 {
				return req, fmt.Errorf("empty topic filter")
			}
			tf := make(rpc.TopicFilter, 0, len(pattern))
			for _, seg := range pattern {
				sf := rpc.SegmentFilter{ScVal: seg.Value}
				if seg.Wildcard != "" {
					wildcard := seg.Wildcard
					sf = rpc.SegmentFilter{Wildcard: &wildcard}
				}
				tf = append(tf, sf)
			}
			rf.Topics = append(rf.Topics, tf)
		}
		req.Filters = append(req.Filters, rf)
	}

	if err := req.Valid(maxEventsLimit); err != nil {
		return req, err
	}
	return req, nil
}

// GetEvents queries contract events over the Soroban RPC getEvents method and
// returns them with topics and values decoded into xdr.ScVal.
func (c *Client) GetEvents(ctx context.Context, req EventsRequest) (*EventsResult, error) {
	rpcReq, err := req.toRPC()
	if err != nil {
		return nil, err
	}

	resp, err := c.rpc.GetEvents(ctx, rpcReq)
	if err != nil {
		return nil, fmt.Errorf("getEvents failed: %w", err)
	}

	result := &EventsResult{
		Cursor:       resp.Cursor,
		LatestLedger: resp.LatestLedger,
		OldestLedger: resp.OldestLedger,
	}
	for _, ev := range resp.Events {
		decoded, err := decodeEventInfo(ev)
		if err != nil {
			return nil, err
		}
		result.Events = append(result.Events, decoded)
	}
	return result, nil
}

func decodeEventInfo(ev rpc.EventInfo) (ContractEvent, error) {
	out := ContractEvent{
		Type:           ev.EventType,
		Ledger:         uint32(ev.Ledger),
		LedgerClosedAt: ev.LedgerClosedAt,
		ContractID:     ev.ContractID,
		ID:             ev.ID,
		TxHash:         ev.TransactionHash,
		TxIndex:        ev.TxIndex,
		OpIndex:        ev.OpIndex,
	}
	for _, topicB64 := range ev.TopicXDR {
		var topic xdr.ScVal
		if err := xdr.SafeUnmarshalBase64(topicB64, &topic); err != nil {
			return out, fmt.Errorf("failed to decode event topic: %w", err)
		}
		out.Topics = append(out.Topics, topic)
	}
	if ev.ValueXDR != "" {
		if err := xdr.SafeUnmarshalBase64(ev.ValueXDR, &out.Value); err != nil {
			return out, fmt.Errorf("failed to decode event value: %w", err)
		}
	}
	return out, nil
}

// EventsFromTransactionMeta decodes the contract events embedded in a
// transaction's resultMetaXDR — the field InvokeResult.ResultMetaXDR carries.
// It handles both v3 and v4 transaction meta, covering every protocol version
// that emits Soroban events, and lets a caller confirm what a submitted
// invocation emitted without a separate getEvents round-trip.
func EventsFromTransactionMeta(resultMetaXDR string) ([]ContractEvent, error) {
	var meta xdr.TransactionMeta
	if err := xdr.SafeUnmarshalBase64(resultMetaXDR, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse transaction meta: %w", err)
	}

	var xdrEvents []xdr.ContractEvent
	switch meta.V {
	case 3:
		v3, ok := meta.GetV3()
		if !ok || v3.SorobanMeta == nil {
			return nil, nil
		}
		xdrEvents = v3.SorobanMeta.Events
	case 4:
		v4, ok := meta.GetV4()
		if !ok {
			return nil, nil
		}
		for _, tev := range v4.Events {
			xdrEvents = append(xdrEvents, tev.Event)
		}
	default:
		return nil, fmt.Errorf("transaction meta v%d does not carry Soroban events", meta.V)
	}

	events := make([]ContractEvent, 0, len(xdrEvents))
	for _, ev := range xdrEvents {
		events = append(events, contractEventFromXDR(ev))
	}
	return events, nil
}

func contractEventFromXDR(ev xdr.ContractEvent) ContractEvent {
	out := ContractEvent{
		Type: rpc.GetEventTypeFromEventTypeXDR()[ev.Type],
	}
	if ev.ContractId != nil {
		if id, err := strkey.Encode(strkey.VersionByteContract, ev.ContractId[:]); err == nil {
			out.ContractID = id
		}
	}
	if v0, ok := ev.Body.GetV0(); ok {
		out.Topics = v0.Topics
		out.Value = v0.Data
	}
	return out
}
