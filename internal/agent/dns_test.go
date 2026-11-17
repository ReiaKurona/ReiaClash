package agent

import (
	"fmt"
	"testing"
)

func TestDNSHistoryIsBoundedAndClearable(t *testing.T) {
	app := &App{}
	for i := 0; i < 240; i++ {
		app.RecordDNS(DNSRecord{Source: "kernel", Payload: fmt.Sprintf("[DNS] query %d", i)})
	}
	records, _ := app.DNSHistory()
	if len(records) != 200 || records[0].ID != 41 || records[199].ID != 240 {
		t.Fatal("incorrect DNS retention")
	}
	app.ClearDNSHistory()
	records, _ = app.DNSHistory()
	if len(records) != 0 {
		t.Fatal("clear failed")
	}
	app.RecordDNS(DNSRecord{Source: "manual", Name: "example.com"})
	records, _ = app.DNSHistory()
	if records[0].ID != 241 {
		t.Fatal("IDs were reused after clearing")
	}
}
