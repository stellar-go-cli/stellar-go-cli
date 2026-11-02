package soroban

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// rpcStub serves a canned getEvents response and records the request params.
func rpcStub(t *testing.T, result any, reqParams *json.RawMessage) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		if req.Method != "getEvents" {
			t.Errorf("unexpected method %q", req.Method)
		}
		if reqParams != nil {
			*reqParams = req.Params
		}
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(req.ID),
			"result":  result,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
}

func TestGetEvents(t *testing.T) {
	contractID := "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"
	value := ScvMap(ScvPair(ScvSymbol("amount"), ScvU64(1_000)))
	valueB64, err := xdr.MarshalBase64(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	topicB64, err := xdr.MarshalBase64(ScvSymbol("commit"))
	if err != nil {
		t.Fatalf("marshal topic: %v", err)
	}

	canned := map[string]any{
		"events": []map[string]any{
			{
				"type":             "contract",
				"ledger":           12345,
				"ledgerClosedAt":   "2026-10-22T12:00:00Z",
				"contractId":       contractID,
				"id":               "0000530177047470080-0000000001",
				"operationIndex":   0,
				"transactionIndex": 1,
				"txHash":           "abc123",
				"topic":            []string{topicB64},
				"value":            valueB64,
			},
		},
		"cursor":       "0000530177047470080-0000000001",
		"latestLedger": 12400,
		"oldestLedger": 11000,
	}

	var params json.RawMessage
	srv := rpcStub(t, canned, &params)
	defer srv.Close()

	client := NewClient(srv.URL, "Test SDF Network ; September 2015")
	defer client.Close()

	res, err := client.GetEvents(context.Background(), EventsRequest{
		StartLedger: 12000,
		EndLedger:   13000,
		Filters: []EventFilter{{
			Types:       []string{"contract"},
			ContractIDs: []string{contractID},
			Topics:      [][]EventTopicSegment{{TopicSymbol("commit"), TopicAnyMore()}},
		}},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}

	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}
	ev := res.Events[0]
	if ev.ContractID != contractID || ev.TxHash != "abc123" || ev.Ledger != 12345 {
		t.Fatalf("unexpected event metadata: %+v", ev)
	}
	if len(ev.Topics) != 1 || ev.Topics[0].Type != xdr.ScValTypeScvSymbol {
		t.Fatalf("unexpected topics: %+v", ev.Topics)
	}
	if ev.Value.Type != xdr.ScValTypeScvMap {
		t.Fatalf("expected map value, got %s", ev.Value.Type)
	}
	if res.Cursor != "0000530177047470080-0000000001" || res.LatestLedger != 12400 || res.OldestLedger != 11000 {
		t.Fatalf("unexpected page metadata: %+v", res)
	}

	// The wire request must carry the filters and pagination through.
	var wire struct {
		StartLedger uint32 `json:"startLedger"`
		EndLedger   uint32 `json:"endLedger"`
		Filters     []struct {
			Type        string     `json:"type"`
			ContractIDs []string   `json:"contractIds"`
			Topics      [][]string `json:"topics"`
		} `json:"filters"`
		Pagination struct {
			Limit uint `json:"limit"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(params, &wire); err != nil {
		t.Fatalf("unmarshal wire params: %v", err)
	}
	if wire.StartLedger != 12000 || wire.EndLedger != 13000 || wire.Pagination.Limit != 10 {
		t.Fatalf("unexpected wire request: %s", params)
	}
	if len(wire.Filters) != 1 || wire.Filters[0].Type != "contract" {
		t.Fatalf("unexpected wire filters: %s", params)
	}
	if len(wire.Filters[0].ContractIDs) != 1 || wire.Filters[0].ContractIDs[0] != contractID {
		t.Fatalf("unexpected wire contract IDs: %s", params)
	}
	if len(wire.Filters[0].Topics) != 1 || len(wire.Filters[0].Topics[0]) != 2 {
		t.Fatalf("unexpected wire topics: %s", params)
	}
	if wire.Filters[0].Topics[0][1] != "**" {
		t.Fatalf("expected trailing wildcard, got %s", params)
	}
}

func TestGetEventsValidation(t *testing.T) {
	client := NewClientForNetwork("stellar-testnet")
	defer client.Close()

	// Cursor and ledger range are mutually exclusive.
	if _, err := client.GetEvents(context.Background(), EventsRequest{
		StartLedger: 100,
		Cursor:      "0000530177047470080-0000000001",
	}); err == nil {
		t.Fatal("expected error for cursor + startLedger")
	}

	// Cursor must parse.
	if _, err := client.GetEvents(context.Background(), EventsRequest{Cursor: "bogus"}); err == nil {
		t.Fatal("expected error for invalid cursor")
	}

	// Invalid contract ID rejected by filter validation.
	if _, err := client.GetEvents(context.Background(), EventsRequest{
		StartLedger: 100,
		Filters:     []EventFilter{{ContractIDs: []string{"not-a-contract"}}},
	}); err == nil {
		t.Fatal("expected error for invalid contract ID")
	}
}

func TestEventsFromTransactionMetaV3(t *testing.T) {
	contractAddr, err := ParseContractID("CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC")
	if err != nil {
		t.Fatalf("parse contract id: %v", err)
	}
	data := ScvMap(ScvPair(ScvSymbol("amount"), ScvU64(1_000)))
	ev := xdr.ContractEvent{
		Type:       xdr.ContractEventTypeContract,
		ContractId: contractAddr.ContractId,
		Body: xdr.ContractEventBody{
			V: 0,
			V0: &xdr.ContractEventV0{
				Topics: []xdr.ScVal{ScvSymbol("commit")},
				Data:   data,
			},
		},
	}
	meta := xdr.TransactionMeta{
		V: 3,
		V3: &xdr.TransactionMetaV3{
			SorobanMeta: &xdr.SorobanTransactionMeta{
				Events:      []xdr.ContractEvent{ev},
				ReturnValue: ScvBool(true),
			},
		},
	}
	metaB64, err := xdr.MarshalBase64(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}

	events, err := EventsFromTransactionMeta(metaB64)
	if err != nil {
		t.Fatalf("EventsFromTransactionMeta: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.Type != "contract" {
		t.Fatalf("unexpected event type %q", got.Type)
	}
	if got.ContractID != "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC" {
		t.Fatalf("unexpected contract id %q", got.ContractID)
	}
	if len(got.Topics) != 1 || got.Topics[0].Type != xdr.ScValTypeScvSymbol {
		t.Fatalf("unexpected topics: %+v", got.Topics)
	}
	gotB64, wantB64 := mustMarshalB64(t, got.Value), mustMarshalB64(t, data)
	if gotB64 != wantB64 {
		t.Fatalf("event value mismatch:\n got %s\nwant %s", gotB64, wantB64)
	}
}

func TestEventsFromTransactionMetaV4(t *testing.T) {
	ev := xdr.ContractEvent{
		Type: xdr.ContractEventTypeContract,
		Body: xdr.ContractEventBody{
			V: 0,
			V0: &xdr.ContractEventV0{
				Topics: []xdr.ScVal{ScvSymbol("stake_chg")},
				Data:   ScvI64(-250),
			},
		},
	}
	meta := xdr.TransactionMeta{
		V: 4,
		V4: &xdr.TransactionMetaV4{
			Events: []xdr.TransactionEvent{
				{Event: ev},
			},
		},
	}
	metaB64, err := xdr.MarshalBase64(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	events, err := EventsFromTransactionMeta(metaB64)
	if err != nil {
		t.Fatalf("EventsFromTransactionMeta: %v", err)
	}
	if len(events) != 1 || len(events[0].Topics) != 1 {
		t.Fatalf("unexpected events: %+v", events)
	}
}
