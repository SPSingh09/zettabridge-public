package livebrokers

import (
	"strings"
	"testing"
)

func TestFindSecurityInInstrumentCSV(t *testing.T) {
	csv := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SM_SYMBOL_NAME,SEM_CUSTOM_SYMBOL
NSE,E,11536,TCS,TCS
NSE,E,2885,RELIANCE,RELIANCE`

	id, ts, ok := findSecurityInInstrumentCSV(strings.NewReader(csv), "TCS")
	if !ok || id != "11536" || ts != "TCS" {
		t.Fatalf("got id=%q ts=%q ok=%v", id, ts, ok)
	}

	id, ts, ok = findSecurityInInstrumentCSV(strings.NewReader(csv), "RELIANCE")
	if !ok || id != "2885" {
		t.Fatalf("got id=%q ts=%q", id, ts)
	}

	_, _, ok = findSecurityInInstrumentCSV(strings.NewReader(csv), "UNKNOWN")
	if ok {
		t.Fatal("expected miss")
	}
}

func TestDhanExchangeSegment(t *testing.T) {
	if dhanExchangeSegment("NSE") != "NSE_EQ" {
		t.Fatal("NSE")
	}
	if dhanExchangeSegment("BSE") != "BSE_EQ" {
		t.Fatal("BSE")
	}
}

func TestDhanProductType(t *testing.T) {
	if dhanProductType("MIS") != "INTRADAY" {
		t.Fatal("MIS")
	}
	if dhanProductType("CNC") != "CNC" {
		t.Fatal("CNC")
	}
	if dhanProductType("NRML") != "MARGIN" {
		t.Fatal("NRML")
	}
}
